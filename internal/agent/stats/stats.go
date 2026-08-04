// Package stats 通过 Xray gRPC StatsService 采集用户流量。
// 实现模型对照《xray-api-探索.md》§4（3x-ui GetTraffic 轮询模式）：
// QueryStats 全量拉取 → 维护 last 基线 → 差值 = 本周期增量（xray 重启归零视为 0）。
package stats

import (
	"context"
	"fmt"
	"regexp"
	"sync"
	"time"

	statsService "github.com/xtls/xray-core/app/stats/command"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Entry 单个用户本周期流量增量。
type Entry struct {
	Email string
	Up    int64
	Down  int64
}

// user 计数器命名：user>>>email>>>traffic>>>(uplink|downlink)
var userRe = regexp.MustCompile(`^user>>>(.+?)>>>traffic>>>(uplink|downlink)$`)

// Collector 采集 Xray stats。
type Collector struct {
	apiAddr string
	conn    *grpc.ClientConn
	client  statsService.StatsServiceClient

	mu       sync.Mutex
	last     map[string]int64 // 计数器名 → 上次累计值
	baseline bool             // 是否已建立基线
}

// New 构造采集器（apiAddr 如 127.0.0.1:10085）。
func New(apiAddr string) *Collector {
	return &Collector{apiAddr: apiAddr, last: make(map[string]int64)}
}

// Connect 建立 gRPC 连接（xray 重启后需重新连接）。
func (c *Collector) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connectLocked()
}

// connectLocked 建立 gRPC 连接（调用方须持有 mu）。
func (c *Collector) connectLocked() error {
	if c.client != nil {
		return nil
	}
	conn, err := grpc.NewClient(c.apiAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("连接 xray stats 失败: %w", err)
	}
	c.conn = conn
	c.client = statsService.NewStatsServiceClient(conn)
	// 注意：不重置 last/baseline —— 临时连接失败后重连应继续差值；
	// xray 重启计数器归零时，差值逻辑会自然处理（cur < prev → delta = cur）。
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
	}
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