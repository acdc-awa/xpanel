package xray_test

// 四层直通用**面板生成的配置**跑真实 xray，对 TCP/UDP 做数据往返。
// 为什么必须做数据往返：`xray -test` 对未知字段静默忽略，目标写错位置（入站 vs 出站）照样通过，
// 只有真连一次才能证明"入站只监听、目标由派生 freedom 出站的 redirect 承担"成立。
// 两个子用例构成对照：透传关闭（明文载荷）与透传开启（PROXY v2 头 + 载荷）。
// xray 二进制缺失时自动跳过（Linux CI 上必然跳过，因为仓库只带 Windows 二进制）。

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/acdc-awa/xpanel/internal/master/xray"
	"github.com/acdc-awa/xpanel/internal/models"
)

const proxyV2Signature = "\r\n\r\n\x00\r\nQUIT\n"

// freeDualProtoPort 在稳定区间里挑一个 **TCP 与 UDP 都能绑定** 的端口。
// 不能只探 TCP：Windows 有保留/排除端口段，TCP 能绑的口 UDP 可能直接拒绝
// （bind: An attempt was made to access a socket in a way forbidden by its access permissions），
// 而直通入站的 network 是 tcp,udp，两协议都要绑得上。
func freeDualProtoPort(t *testing.T) int {
	t.Helper()
	for port := 45000; port < 55000; port++ {
		l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			continue
		}
		u, err := net.ListenPacket("udp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			l.Close()
			continue
		}
		u.Close()
		l.Close()
		return port
	}
	t.Fatal("找不到 TCP/UDP 都能绑定的端口")
	return 0
}

func TestTunnelInbound_RedirectOutbound_DataRoundtrip(t *testing.T) {
	echoPort := freeDualProtoPort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", echoPort)

	tcpLn, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("tcp echo listen: %v", err)
	}
	defer tcpLn.Close()
	go func() {
		for {
			c, err := tcpLn.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 4096)
				for {
					n, err := c.Read(buf)
					if n > 0 {
						_, _ = c.Write(append([]byte("ECHO|"), buf[:n]...))
					}
					if err != nil {
						return
					}
				}
			}(c)
		}
	}()

	udpEcho, err := net.ListenPacket("udp", addr)
	if err != nil {
		t.Fatalf("udp echo listen: %v", err)
	}
	defer udpEcho.Close()
	go func() {
		buf := make([]byte, 4096)
		for {
			n, from, err := udpEcho.ReadFrom(buf)
			if err != nil {
				return
			}
			_, _ = udpEcho.WriteTo(append([]byte("UECHO|"), buf[:n]...), from)
		}
	}()

	for _, tc := range []struct {
		name   string
		stream string
		wantPP bool
	}{
		{"透传关闭", `{"proxy_protocol":false}`, false},
		{"透传开启（缺省）", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tunPort := freeDualProtoPort(t)
			inbounds := []models.Inbound{{
				ID: 1, ServerID: 1, Tag: "tunnel-e2e", Protocol: models.ProtocolDokodemo,
				Port: tunPort, Type: models.InboundTypeTunnel,
				TargetAddress: "127.0.0.1", TargetPort: echoPort,
				StreamSettings: tc.stream,
				Enabled:        true,
			}}
			obs, rules := tunnelLedgerRows(t, inbounds[0], nil)
			raw, err := xray.Generate(inbounds, obs, rules, nil, nil, "", "")
			if err != nil {
				t.Fatalf("Generate failed: %v", err)
			}
			// 结构断言走解析，不用字符串匹配（缩进/空格会变）
			var root map[string]any
			if err := json.Unmarshal(raw, &root); err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}
			inb0 := root["inbounds"].([]any)[0].(map[string]any)
			st, _ := inb0["settings"].(map[string]any)
			if _, has := st["address"]; has {
				t.Errorf("入站不应再携带目标 address: %v", st)
			}
			if st["followRedirect"] != false {
				t.Errorf("followRedirect = %v, want false", st["followRedirect"])
			}
			var obSettings map[string]any
			for _, ob := range root["outbounds"].([]any) {
				m := ob.(map[string]any)
				if m["tag"] == "out-tunnel-e2e" {
					obSettings, _ = m["settings"].(map[string]any)
				}
			}
			if obSettings == nil {
				t.Fatalf("未找到派生出口 out-tunnel-e2e: %s", string(raw))
			}
			wantRedirect := fmt.Sprintf("127.0.0.1:%d", echoPort)
			if obSettings["redirect"] != wantRedirect {
				t.Errorf("redirect = %v, want %v（目标由出站承担）", obSettings["redirect"], wantRedirect)
			}
			if _, hasPP := obSettings["proxyProtocol"]; hasPP != tc.wantPP {
				t.Errorf("proxyProtocol 存在性 = %v, want %v", hasPP, tc.wantPP)
			}

			xrayBin := filepath.Join("..", "..", "..", "tools", "xray-windows-64", "xray.exe")
			if _, err := os.Stat(xrayBin); err != nil {
				t.Skipf("xray 二进制缺失: %v", err)
			}
			// cmd.Dir 会切到临时目录，二进制路径必须先转绝对路径
			xrayBin, err = filepath.Abs(xrayBin)
			if err != nil {
				t.Fatalf("abs: %v", err)
			}
			cfgPath := filepath.Join(t.TempDir(), "tunnel-e2e.json") // 必须 .json 结尾：xray 按扩展名判格式
			if err := os.WriteFile(cfgPath, raw, 0644); err != nil {
				t.Fatalf("写配置失败: %v", err)
			}
			var logs strings.Builder
			cmd := exec.Command(xrayBin, "run", "-c", cfgPath)
			cmd.Dir = filepath.Dir(cfgPath)
			cmd.Stdout = &logs
			cmd.Stderr = &logs
			if err := cmd.Start(); err != nil {
				t.Fatalf("启动 xray 失败: %v", err)
			}
			defer func() {
				_ = cmd.Process.Kill()
				_, _ = cmd.Process.Wait()
			}()

			deadline := time.Now().Add(15 * time.Second)
			for {
				c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", tunPort), 300*time.Millisecond)
				if err == nil {
					c.Close()
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("xray 未监听 %d；日志: %s", tunPort, logs.String())
				}
				time.Sleep(100 * time.Millisecond)
			}

			target := fmt.Sprintf("127.0.0.1:%d", tunPort)
			tcpReply, tcpOK := dualProtoRoundtrip(t, "tcp", target, "PING", 6*time.Second)
			udpReply, udpOK := dualProtoRoundtrip(t, "udp", target, "UECHO|PING", 6*time.Second)

			if tc.wantPP {
				// PROXY v2 头夹在 ECHO| 与载荷之间：只能断言"头在、载荷在"
				if !tcpOK || !strings.Contains(tcpReply, proxyV2Signature) || !strings.HasSuffix(tcpReply, "PING") {
					t.Errorf("TCP（透传开启）往返失败: %q；日志: %s", tcpReply, logs.String())
				}
				if !udpOK || !strings.Contains(udpReply, proxyV2Signature) {
					t.Errorf("UDP（透传开启）往返失败: %q", udpReply)
				}
			} else {
				if tcpReply != "ECHO|PING" {
					t.Errorf("TCP（透传关闭）往返 = %q, want %q", tcpReply, "ECHO|PING")
				}
				if !udpOK {
					t.Errorf("UDP（透传关闭）往返失败: %q", udpReply)
				}
			}
			t.Logf("[%s] TCP=%q UDP=%q", tc.name, tcpReply, udpReply)
		})
	}
}

func dualProtoRoundtrip(t *testing.T, network, target, want string, timeout time.Duration) (string, bool) {
	t.Helper()
	c, err := net.Dial(network, target)
	if err != nil {
		return "dial err: " + err.Error(), false
	}
	defer c.Close()
	if _, err := c.Write([]byte("PING")); err != nil {
		return "write err: " + err.Error(), false
	}
	_ = c.SetReadDeadline(time.Now().Add(timeout))
	buf := make([]byte, 4096)
	acc := ""
	for {
		n, err := c.Read(buf)
		if n > 0 {
			acc += string(buf[:n])
			if strings.Contains(acc, want) {
				return acc, true
			}
		}
		if err != nil {
			return acc, false
		}
	}
}

// tunnelLedgerRows 按 C2（2026-09-28）之后的形态构造直通的两行派生对象：freedom 出口
// （tag = out-<入站tag>，目标由 settings.redirect 承担）与转发规则。
// 生产里这两行由 services.EnsureTunnelDerivedObjects 落库并随直通维护；本包只验证"生成器消费它们"
// 以及"入站只监听"——落库/对账/级联由 services 与 api 的测试覆盖。
func tunnelLedgerRows(t *testing.T, inb models.Inbound, ctx *xray.GenerateContext) ([]models.ServerOutbound, []models.ServerRoutingRule) {
	t.Helper()
	host, port, ok := xray.TunnelTarget(&inb, ctx)
	if !ok {
		t.Fatalf("夹具直通 %s 没有目标（草稿态不该构造派生行）", inb.Tag)
	}
	settings, err := xray.TunnelOutboundSettings(host, port, xray.TunnelSendProxyProtocol(&inb))
	if err != nil {
		t.Fatalf("构造派生出口 settings 失败: %v", err)
	}
	tag := xray.TunnelOutboundTag(inb.Tag)
	return []models.ServerOutbound{{
			ServerID:       inb.ServerID,
			Tag:            tag,
			Protocol:       "freedom",
			SettingsJSON:   settings,
			Enabled:        true,
			Priority:       models.TunnelOutboundPriority,
			OwnerInboundID: inb.ID,
		}}, []models.ServerRoutingRule{{
			ServerID:       inb.ServerID,
			OutboundTag:    tag,
			InboundTag:     inb.Tag,
			Enabled:        true,
			Priority:       models.TunnelRulePriority,
			OwnerInboundID: inb.ID,
		}}
}

// TestTunnelRule_AdminRuleCanPrecede 验证 C2 的核心收益：管理员终于能把规则排到直通规则之前。
//
// C1 之前（实测 t7_tunnel_rule_order.py）：直通规则由生成器固定注入在全部节点规则之前，
// 管理员按 tag 封禁直通的规则**永远不可达**（数据往返照样通）。C2 起直通规则是库内真行、
// 优先级 -100，管理员把自己的规则设成 -200 就排在它前面 ⇒ 封禁生效。
//
// 两个子用例构成反向对照（同一份配置，只差优先级）：
//   - 封禁规则优先级 0（在直通规则之后，等价于历史行为）→ 数据往返必须**通**
//   - 封禁规则优先级 -200（排在直通规则之前）→ 数据往返必须**不通**
func TestTunnelRule_AdminRuleCanPrecede(t *testing.T) {
	xrayBin := filepath.Join("..", "..", "..", "tools", "xray-windows-64", "xray.exe")
	if _, err := os.Stat(xrayBin); err != nil {
		t.Skipf("xray 二进制缺失: %v", err)
	}
	xrayBin, err := filepath.Abs(xrayBin)
	if err != nil {
		t.Fatal(err)
	}

	echoPort := freeDualProtoPort(t)
	tcpLn, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", echoPort))
	if err != nil {
		t.Fatalf("tcp echo listen: %v", err)
	}
	defer tcpLn.Close()
	go func() {
		for {
			c, err := tcpLn.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 4096)
				for {
					n, err := c.Read(buf)
					if n > 0 {
						_, _ = c.Write(append([]byte("ECHO|"), buf[:n]...))
					}
					if err != nil {
						return
					}
				}
			}(c)
		}
	}()

	for _, tc := range []struct {
		name          string
		blockPrio     int
		wantRoundtrip bool
	}{
		{"管理员规则在直通之后（优先级 0，历史行为）", 0, true},
		{"管理员规则排在直通之前（优先级 -200）", -200, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tunPort := freeDualProtoPort(t)
			inb := models.Inbound{
				ID: 1, ServerID: 1, Tag: "tunnel-prio", Protocol: models.ProtocolDokodemo,
				Port: tunPort, Type: models.InboundTypeTunnel,
				TargetAddress: "127.0.0.1", TargetPort: echoPort,
				// 关闭 PROXY Protocol v2：本用例只验证"规则顺序"，开着会让目标先收到一个二进制头，
				// 使回显数据被切成两段（C1 的往返回归里踩过同一个坑）
				StreamSettings: `{"proxy_protocol":false}`,
				Enabled:        true,
			}
			obs, rules := tunnelLedgerRows(t, inb, nil)
			rules = append(rules, models.ServerRoutingRule{
				ServerID: 1, OutboundTag: "blocked", InboundTag: "tunnel-prio",
				Enabled: true, Priority: tc.blockPrio, Remark: "管理员按 tag 封禁该直通",
			})
			raw, err := xray.Generate([]models.Inbound{inb}, obs, rules, nil, nil, "", "")
			if err != nil {
				t.Fatalf("Generate failed: %v", err)
			}
			cfgPath := filepath.Join(t.TempDir(), "prio.json") // .json 结尾：xray 按扩展名判格式
			if err := os.WriteFile(cfgPath, raw, 0644); err != nil {
				t.Fatal(err)
			}
			var logs strings.Builder
			cmd := exec.Command(xrayBin, "run", "-c", cfgPath)
			cmd.Dir = filepath.Dir(cfgPath)
			cmd.Stdout = &logs
			cmd.Stderr = &logs
			if err := cmd.Start(); err != nil {
				t.Fatalf("启动 xray 失败: %v", err)
			}
			defer func() {
				_ = cmd.Process.Kill()
				_, _ = cmd.Process.Wait()
			}()
			deadline := time.Now().Add(15 * time.Second)
			for {
				c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", tunPort), 300*time.Millisecond)
				if err == nil {
					c.Close()
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("xray 未监听 %d；日志: %s", tunPort, logs.String())
				}
				time.Sleep(100 * time.Millisecond)
			}

			got := "no-roundtrip"
			conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", tunPort), 4*time.Second)
			if err != nil {
				got = fmt.Sprintf("dial-failed: %v", err)
			} else {
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
				if _, err := conn.Write([]byte("PING")); err == nil {
					// 读到看见回显负载为止（不按固定字节数读，也不要求两段连续）
					total := ""
					buf := make([]byte, 256)
					for i := 0; i < 8; i++ {
						n, err := conn.Read(buf)
						if n > 0 {
							total += string(buf[:n])
						}
						if strings.Contains(total, "ECHO|") && strings.Contains(total, "PING") {
							got = "roundtrip-ok"
							break
						}
						if err != nil {
							break
						}
					}
				}
			}
			if tc.wantRoundtrip && got != "roundtrip-ok" {
				t.Errorf("默认优先级下直通应照常可用，实际 %s（日志: %s）", got, logs.String())
			}
			if !tc.wantRoundtrip && got == "roundtrip-ok" {
				t.Errorf("封禁规则排在直通之前却没生效：直通仍然可用（C2 的排序能力没落地）")
			}
			t.Logf("%s → %s", tc.name, got)
		})
	}
}
