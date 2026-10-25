package services

// C2 头号验收（常驻）：把四层直通的派生对象从"生成期现场派生"改成"库内真行"之后，
// 生成结果必须**逐字节不变**——否则就不是换存储位置，而是换行为。
//
// 两条路各自独立地喂给生成器，要求输出完全相同：
//   A（C1 等价位）：按老代码的内联表达式在内存里构造派生出口与转发规则（tag = out-<入站tag>，
//     目标写 settings.redirect，未显式关闭时带 proxyProtocol: 2；规则优先级 -100 排在 BT 之前）。
//   B（C2 落库位）：调 EnsureTunnelDerivedObjects 落库 → 按生产同序取数（loadOutboundsAndRules）→ 生成。
// A 侧的表达式是**故意重复**的生产逻辑，用来自证"落库没有改变任何一个字节"。
//
// 夹具覆盖四类直通：手动目标 / 连线目标 / 通道直通 / 草稿（草稿不产生任何派生行）。
import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/master/xray"
	"github.com/acdc-awa/xpanel/internal/models"
)

func equivDB(t *testing.T) *gorm.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := gorm.Open(sqlite.Open(filepath.Join(dir, "eq.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := models.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

func seedEquiv(t *testing.T, db *gorm.DB) uint64 {
	t.Helper()

	edge := models.Server{ID: 1, Name: "edge", Host: "edge.example.com", NodeID: "node-edge", DefaultOutboundTag: "direct"}
	exit := models.Server{ID: 2, Name: "exit", Host: "exit.example.com", NodeID: "node-exit", DefaultOutboundTag: "direct"}
	if err := db.Create(&edge).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&exit).Error; err != nil {
		t.Fatal(err)
	}

	landing := models.Inbound{
		ID: 201, ServerID: 2, Tag: "vless-land", Protocol: models.ProtocolVless, Port: 443,
		Type: models.InboundTypeUser, Enabled: true, InternalUUID: "11111111-2222-3333-4444-555555555555",
	}
	if err := db.Create(&landing).Error; err != nil {
		t.Fatal(err)
	}
	landingID := landing.ID

	inbounds := []models.Inbound{
		{ID: 101, ServerID: 1, Tag: "tunnel-manual", Protocol: models.ProtocolTunnel, Port: 20001,
			Type: models.InboundTypeTunnel, TargetAddress: "target.example.com", TargetPort: 3306,
			StreamSettings: `{"proxy_protocol":true}`, Enabled: true},
		{ID: 102, ServerID: 1, Tag: "tunnel-linked", Protocol: models.ProtocolTunnel, Port: 20002,
			Type: models.InboundTypeTunnel, TargetInboundID: &landingID,
			StreamSettings: `{"proxy_protocol":false}`, Enabled: true},
		{ID: 103, ServerID: 1, Tag: "chan-tunnel-20003", Protocol: models.ProtocolTunnel, Port: 20003,
			Type: models.InboundTypeChannel, TargetAddress: "db.remote.com", TargetPort: 5432,
			StreamSettings: `{"proxy_protocol":true}`, Enabled: true},
		{ID: 104, ServerID: 1, Tag: "tunnel-draft", Protocol: models.ProtocolTunnel, Port: 20004,
			Type: models.InboundTypeTunnel, Enabled: true},
		{ID: 105, ServerID: 1, Tag: "vless-in-443", Protocol: models.ProtocolVless, Port: 443,
			Type: models.InboundTypeUser, Enabled: true,
			SettingsJSON: `{"clients":[],"decryption":"none"}`},
	}
	for i := range inbounds {
		if err := db.Create(&inbounds[i]).Error; err != nil {
			t.Fatal(err)
		}
	}

	outbounds := []models.ServerOutbound{
		{ServerID: 1, Tag: "direct", Protocol: "freedom", SettingsJSON: models.DefaultFreedomDirectSettingsJSON, Enabled: true, Priority: 0},
		{ServerID: 1, Tag: "blocked", Protocol: "blackhole", SettingsJSON: `{"response":{"type":"none"}}`, Enabled: true, Priority: 99},
		{ServerID: 1, Tag: "warp", Protocol: "freedom", SettingsJSON: `{}`, Enabled: true, Priority: 0},
	}
	for i := range outbounds {
		if err := db.Create(&outbounds[i]).Error; err != nil {
			t.Fatal(err)
		}
	}

	rules := []models.ServerRoutingRule{
		{ServerID: 1, OutboundTag: "warp", InboundTag: "vless-in-443", Enabled: true, Priority: 0, Remark: "用户入站走 warp"},
		{ServerID: 1, OutboundTag: "blocked", Domain: "ads.example.com", Enabled: true, Priority: 5, Remark: "广告域名阻断"},
	}
	for i := range rules {
		if err := db.Create(&rules[i]).Error; err != nil {
			t.Fatal(err)
		}
	}
	return 1
}

func TestTunnelDerived_C2Equivalence(t *testing.T) {
	db := equivDB(t)
	serverID := seedEquiv(t, db)

	dbOutbounds, dbRules, err := (&ConfigService{DB: db}).loadOutboundsAndRules(serverID)
	if err != nil {
		t.Fatal(err)
	}
	var allInbounds []models.Inbound
	if err := db.Where("server_id = ?", serverID).Find(&allInbounds).Error; err != nil {
		t.Fatal(err)
	}
	topoA, err := buildGenerateContext(db, nil, allInbounds, dbOutbounds)
	if err != nil {
		t.Fatal(err)
	}
	var enabledInbounds []models.Inbound
	if err := db.Where("server_id = ? AND enabled = ?", serverID, true).Find(&enabledInbounds).Error; err != nil {
		t.Fatal(err)
	}
	enabledInbounds = FilterAvailableInbounds(enabledInbounds)

	// A 路：把"C1 生成器曾在内存里现场派生"的两样东西按同样的字节构造出来喂给生成器
	// （出站追加在库内出站之后、规则 Priority=-100 排在 BT 之前——与 C1 的插入位置一致）
	var derivedObs []models.ServerOutbound
	var derivedRules []models.ServerRoutingRule
	for i := range enabledInbounds {
		inb := enabledInbounds[i]
		if !IsTunnelInbound(&inb) {
			continue
		}
		host, port, ok := xray.TunnelTarget(&inb, topoA)
		if !ok {
			continue
		}
		obSettings := map[string]any{"redirect": fmt.Sprintf("%s:%d", host, port)}
		if xray.TunnelSendProxyProtocol(&inb) {
			obSettings["proxyProtocol"] = 2
		}
		obJSON, err := json.Marshal(obSettings)
		if err != nil {
			t.Fatal(err)
		}
		tag := xray.TunnelOutboundTag(inb.Tag)
		derivedObs = append(derivedObs, models.ServerOutbound{
			ServerID: serverID, Tag: tag, Protocol: "freedom", SettingsJSON: string(obJSON),
			Enabled: inb.Enabled, Priority: models.TunnelOutboundPriority, OwnerInboundID: inb.ID,
		})
		derivedRules = append(derivedRules, models.ServerRoutingRule{
			ServerID: serverID, OutboundTag: tag, InboundTag: inb.Tag,
			Enabled: inb.Enabled, Priority: models.TunnelRulePriority, OwnerInboundID: inb.ID,
		})
	}
	// 规则按 (priority, id) 排序——与 loadOutboundsAndRules 的取数顺序一致（派生规则 -100 排最前）
	combinedRules := append(append([]models.ServerRoutingRule{}, dbRules...), derivedRules...)
	sort.SliceStable(combinedRules, func(i, j int) bool { return combinedRules[i].Priority < combinedRules[j].Priority })
	rawA, err := xray.Generate(enabledInbounds, append(append([]models.ServerOutbound{}, dbOutbounds...), derivedObs...),
		combinedRules, nil, topoA, "direct", "")
	if err != nil {
		t.Fatal(err)
	}

	// B 路：对账落库 → 按生产同序取数 → 生成（C2 之后真实路径）
	if err := EnsureTunnelDerivedObjects(db, serverID); err != nil {
		t.Fatal(err)
	}
	var inbounds []models.Inbound
	if err := db.Where("server_id = ? AND enabled = ?", serverID, true).Find(&inbounds).Error; err != nil {
		t.Fatal(err)
	}
	inbounds = FilterAvailableInbounds(inbounds)
	outbounds, rules, err := (&ConfigService{DB: db}).loadOutboundsAndRules(serverID)
	if err != nil {
		t.Fatal(err)
	}
	topo, err := buildGenerateContext(db, nil, inbounds, outbounds)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := xray.Generate(inbounds, outbounds, rules, nil, topo, "direct", "")
	if err != nil {
		t.Fatal(err)
	}

	if os.Getenv("C2_EQUIV_DUMP") != "" {
		_ = os.WriteFile(filepath.Join(os.TempDir(), "c2-path-a.json"), rawA, 0o644)
		_ = os.WriteFile(filepath.Join(os.TempDir(), "c2-path-b.json"), raw, 0o644)
	}

	if string(rawA) != string(raw) {
		gl := strings.Split(string(rawA), "\n")
		rl := strings.Split(string(raw), "\n")
		for i := 0; i < len(gl) || i < len(rl); i++ {
			g, r := "", ""
			if i < len(gl) {
				g = gl[i]
			}
			if i < len(rl) {
				r = rl[i]
			}
			if g != r {
				t.Errorf("第 %d 行不同:\n  C1 等价位: %s\n  C2 落库位: %s", i+1, g, r)
			}
		}
		t.Fatalf("C2 换存储位置后生成结果变化（%d 字节 vs %d 字节）", len(rawA), len(raw))
	}
	t.Logf("逐字节一致：%d 字节（出站与规则的 tag/settings/顺序全部相同）", len(raw))
}
