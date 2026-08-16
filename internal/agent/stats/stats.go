// Package stats 通过 Xray gRPC StatsService 采集用户流量，并支持 HandlerService 动态更新用户。
// 实现模型对照《xray-api-探索.md》§4（3x-ui GetTraffic 轮询模式）：
// QueryStats 全量拉取 → 维护 last 基线 → 差值 = 本周期增量（xray 重启归零视为 0）。
package stats

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	handlerService "github.com/xtls/xray-core/app/proxyman/command"
	statsService "github.com/xtls/xray-core/app/stats/command"
	xrayProtocol "github.com/xtls/xray-core/common/protocol"
	"github.com/xtls/xray-core/common/serial"
	"github.com/xtls/xray-core/proxy/vless"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/zhx/xray-panel/internal/pkg/protocol"
)

// Entry 单个用户本周期流量增量。
type Entry struct {
	Email string
	Up    int64
	Down  int64
}

// user 计数器命名：user>>>email>>>traffic>>>(uplink|downlink)
var userRe = regexp.MustCompile(`^user>>>(.+?)>>>traffic>>>(uplink|downlink)$`)

// online 计数器命名：user>>>email>>>online（xray stats 中的在线连接计数）。
var onlineRe = regexp.MustCompile(`^user>>>.*>>>online$`)

// Collector 采集 Xray stats 并通过 HandlerService 动态同步用户。
type Collector struct {
	apiAddr string
	conn    *grpc.ClientConn
	client  statsService.StatsServiceClient
	handler handlerService.HandlerServiceClient

	mu           sync.Mutex
	last         map[string]int64                    // 计数器名 → 上次累计值
	baseline     bool                                // 是否已建立基线
	currentUsers map[string]map[string]protocol.User // inboundTag -> email -> protocol.User
	online       int                                 // 最近一次 Collect 观测到的在线用户数
}

// New 构造采集器（apiAddr 如 127.0.0.1:10085）。
func New(apiAddr string) *Collector {
	return &Collector{
		apiAddr:      apiAddr,
		last:         make(map[string]int64),
		currentUsers: make(map[string]map[string]protocol.User),
	}
}

// OnlineUsers 返回最近一次 Collect 观测到的在线用户数（未采集过则 0）。
func (c *Collector) OnlineUsers() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.online
}

// Connect 建立 gRPC 连接（xray 重启后需重新连接）。
func (c *Collector) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connectLocked()
}

// connectLocked 建立 gRPC 连接（调用方须持有 mu）。
func (c *Collector) connectLocked() error {
	if c.client != nil && c.handler != nil {
		return nil
	}
	conn, err := grpc.NewClient(c.apiAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("连接 xray api 失败: %w", err)
	}
	c.conn = conn
	c.client = statsService.NewStatsServiceClient(conn)
	c.handler = handlerService.NewHandlerServiceClient(conn)
	return nil
}

// Close 关闭连接。
func (c *Collector) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
		c.client = nil
		c.handler = nil
	}
}

// ResetUsers 重置内存中的用户缓存（在 Xray 重启后使用）。
func (c *Collector) ResetUsers() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.currentUsers = make(map[string]map[string]protocol.User)
}

// SyncUsers 通过 gRPC HandlerService 增量调整 Xray 中的用户。
func (c *Collector) SyncUsers(ctx context.Context, targetUsers map[string][]protocol.User) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.handler == nil {
		if err := c.connectLocked(); err != nil {
			return err
		}
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if c.currentUsers == nil {
		c.currentUsers = make(map[string]map[string]protocol.User)
	}

	// 1. 遍历 targetUsers 中的每个 Inbound
	for tag, users := range targetUsers {
		existing, ok := c.currentUsers[tag]
		if !ok {
			existing = make(map[string]protocol.User)
			c.currentUsers[tag] = existing
		}

		targetMap := make(map[string]protocol.User, len(users))
		for _, u := range users {
			targetMap[u.Email] = u
		}

		// 移除不在 targetMap 中的用户
		for email := range existing {
			if _, exists := targetMap[email]; !exists {
				remOp := &handlerService.RemoveUserOperation{Email: email}
				req := &handlerService.AlterInboundRequest{
					Tag:       tag,
					Operation: serial.ToTypedMessage(remOp),
				}
				_, err := c.handler.AlterInbound(ctx, req)
				if err != nil {
					if strings.Contains(err.Error(), "not found") {
						log.Printf("agent: gRPC 移除用户 %s (inbound=%s): 用户不存在（跳过）", email, tag)
					} else {
						log.Printf("agent: gRPC 移除用户 %s (inbound=%s) 警告: %v", email, tag, err)
					}
				} else {
					log.Printf("agent: gRPC 动态移除用户 %s (inbound=%s)", email, tag)
				}
				delete(existing, email)
			}
		}

		// 添加或更新 targetMap 中的用户
		for email, newUser := range targetMap {
			oldUser, exists := existing[email]
			if !exists || oldUser.UUID != newUser.UUID || oldUser.Flow != newUser.Flow || oldUser.Level != newUser.Level {
				if exists {
					remOp := &handlerService.RemoveUserOperation{Email: email}
					_, _ = c.handler.AlterInbound(ctx, &handlerService.AlterInboundRequest{
						Tag:       tag,
						Operation: serial.ToTypedMessage(remOp),
					})
				}

				account := &vless.Account{
					Id:   newUser.UUID,
					Flow: newUser.Flow,
				}
				accountTyped := serial.ToTypedMessage(account)

				protoUser := &xrayProtocol.User{
					Email:   newUser.Email,
					Level:   newUser.Level,
					Account: accountTyped,
				}

				addOp := &handlerService.AddUserOperation{User: protoUser}
				req := &handlerService.AlterInboundRequest{
					Tag:       tag,
					Operation: serial.ToTypedMessage(addOp),
				}
				_, err := c.handler.AlterInbound(ctx, req)
				if err != nil {
					if strings.Contains(err.Error(), "already exists") {
						log.Printf("agent: gRPC 添加用户 %s (inbound=%s): 用户已存在（同步状态）", email, tag)
						existing[email] = newUser
					} else {
						log.Printf("agent: gRPC 添加用户 %s (inbound=%s) 失败: %v", email, tag, err)
						return fmt.Errorf("gRPC 添加用户 %s 失败: %w", email, err)
					}
				} else {
					log.Printf("agent: gRPC 动态添加用户 %s (inbound=%s)", email, tag)
					existing[email] = newUser
				}
			}
		}
	}

	// 2. 清理已不在 targetUsers 中的 Inbound 标签
	for tag, existing := range c.currentUsers {
		if _, ok := targetUsers[tag]; !ok {
			for email := range existing {
				remOp := &handlerService.RemoveUserOperation{Email: email}
				_, _ = c.handler.AlterInbound(ctx, &handlerService.AlterInboundRequest{
					Tag:       tag,
					Operation: serial.ToTypedMessage(remOp),
				})
			}
			delete(c.currentUsers, tag)
		}
	}

	return nil
}

// Collect 拉取全量计数器并返回自上次以来的用户流量增量。
// 若 xray 未运行/连接失败返回 error，由调用方决定重连。
func (c *Collector) Collect(ctx context.Context) ([]Entry, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.client == nil {
		if err := c.connectLocked(); err != nil {
			return nil, err
		}
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	resp, err := c.client.QueryStats(ctx, &statsService.QueryStatsRequest{Pattern: "", Reset_: false})
	if err != nil {
		return nil, err
	}

	// ISSUE-11：从 `user>>>email>>>online` 计数器统计在线用户数（值 > 0 视为在线）。
	online := 0
	for _, st := range resp.Stat {
		if onlineRe.MatchString(st.Name) && st.Value > 0 {
			online++
		}
	}
	c.online = online

	// 先建基线：首次调用只记录当前值，不产出增量
	if !c.baseline {
		for _, st := range resp.Stat {
			if userRe.MatchString(st.Name) {
				c.last[st.Name] = st.Value
			}
		}
		c.baseline = true
		return nil, nil
	}

	up := make(map[string]int64)
	down := make(map[string]int64)
	seen := make(map[string]bool)
	for _, st := range resp.Stat {
		m := userRe.FindStringSubmatch(st.Name)
		if m == nil {
			continue
		}
		email, dir := m[1], m[2]
		seen[st.Name] = true
		cur := st.Value
		prev, ok := c.last[st.Name]
		delta := cur
		if ok && cur >= prev {
			delta = cur - prev // 正常增量
		}
		// ok && cur < prev：xray 重启计数器归零，delta=cur 视为从 0 开始
		c.last[st.Name] = cur
		if dir == "uplink" {
			up[email] += delta
		} else {
			down[email] += delta
		}
	}
	// 清理已消失的计数器（删除了用户），防止泄漏
	for name := range c.last {
		if !seen[name] {
			delete(c.last, name)
		}
	}

	entries := make([]Entry, 0, len(up))
	for email, u := range up {
		entries = append(entries, Entry{Email: email, Up: u, Down: down[email]})
	}
	// 仅上报有流量的（down 单独有流量而 up 为 0 的情况）
	for email, d := range down {
		if up[email] == 0 && d > 0 {
			entries = append(entries, Entry{Email: email, Up: 0, Down: d})
		}
	}
	return entries, nil
}
