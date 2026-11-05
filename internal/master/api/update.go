package api

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/acdc-awa/xpanel/internal/pkg/util"
)

// 面板内更新（容器形态自更新）。
//
// 机制：面板内完成 下载 → sha256 校验 → 冒烟试跑 → 备份 → 原子替换 master + web/dist，
// 然后向自身发 SIGTERM 触发既有优雅退出（HTTP/DB/备份全链路关停），进程以 0 退出；
// compose 的 restart: unless-stopped 检测到非人工停止 → 自动拉起容器加载新版本。
// 回滚：替换前保留 master.prev / web.prev，并落 update-pending 标记；
// 容器 entrypoint 仅在「存在 update-pending 且新版本启动失败」时回滚到上一版本重试。
//
// 执行模型（学习 agent handleUpgradeAgent 的后台化 + phase 进度上报，2026-09-03）：
// apply 请求只做参数校验与更新源解析（快），下载/校验/替换全流程转后台 goroutine，
// 阶段进度经 GET /admin/update/status 轮询暴露。后台化根治两个实机问题：
//   - 旧实现用请求 ctx 驱动下载，前端/反代超时掐断请求 = 更新胎死腹中且锁被释放，
//     用户「多点几次有时锁不住」即源于此；现在更新脱离请求生命周期照常执行。
//   - 重复点击由 updateActive CAS 判重入：重复请求幂等返回当前进度而非报错。
// 进程随重启退出，最终成败由前端在面板恢复后比对 current_version 与目标版本判定。

// 更新阶段（与 agent UpgradeProgressPayload.phase 对齐；解压/自检并入 verifying，
// 备份并入 replacing，前端步骤条按「下载→校验→替换→重启」四步渲染）。
const (
	phaseChecking    = "checking"
	phaseDownloading = "downloading"
	phaseVerifying   = "verifying"
	phaseReplacing   = "replacing"
	phaseRestarting  = "restarting"
	phaseFailed      = "failed"
)

// updateProgress 主控自更新进度快照（进程内状态，学习 agent UpgradeProgressPayload）。
type updateProgress struct {
	Running   bool      `json:"running"`
	Phase     string    `json:"phase"`
	From      string    `json:"from_version,omitempty"`
	Target    string    `json:"target_version,omitempty"`
	Message   string    `json:"message"`
	Error     string    `json:"error,omitempty"`
	StartedAt time.Time `json:"started_at,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

var (
	updateActive  atomic.Bool // 更新执行中判重入（apply 幂等；重复请求返回实时进度）
	updateStateMu sync.Mutex  // 保护 updateState
	updateState   updateProgress

	updateCfgCfg = "/app/configs/config.yaml" // 容器形态固定配置路径
)

// updateBuiltinMirrors 下载源候选链（与 deploy/master/install.sh 的 DEFAULT_MIRRORS 同序）。
//
// 旧实现是「单 URL + 单次尝试」：GitHub Release 资产 302 到
// release-assets.githubusercontent.com 后，一次 TLS 握手抖动（默认 Transport 10s 超时）
// 就让整单更新失败，线上表现为「每次发版要点 2–3 次、第一次必报 TLS handshake timeout」。
// 这里与安装脚本、agent 升级器对齐：同源重试 + 跨源回退。
var updateBuiltinMirrors = []string{
	"https://github.com",
	"https://ghproxy.net/https://github.com",
	"https://gh-proxy.com/https://github.com",
	"https://github.moeyy.xyz/https://github.com",
}

const (
	// updateDialTimeout 连接建立超时。
	updateDialTimeout = 10 * time.Second
	// updateTLSTimeout TLS 握手超时：默认 Transport 只有 10s，跨境冷连经常压在边界上。
	updateTLSTimeout = 20 * time.Second
	// updateHeaderTimeout 等待响应头超时（镜像回源慢时不必干等整段预算）。
	updateHeaderTimeout = 30 * time.Second
	// updateDownloadAttempts 同一源内的下载尝试次数（含首次）。
	updateDownloadAttempts = 3
)

// updateRetryBackoff 同一源重试的退避：第 2 次前 2s，第 3 次前 5s。
var updateRetryBackoff = []time.Duration{2 * time.Second, 5 * time.Second}

// updateClient 更新下载专用 client。刻意不用 http.DefaultClient：裸默认值握手只有 10s
// 且没有响应头超时；整体时长仍由 runUpdate 的 ctx（10 分钟）约束。
var updateClient = &http.Client{
	Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   updateDialTimeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   updateTLSTimeout,
		ResponseHeaderTimeout: updateHeaderTimeout,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          4,
		IdleConnTimeout:       90 * time.Second,
	},
}

// updateTagPattern 严格限制版本标签（与 agent 的 TagPattern 同规则），防路径穿越与 URL 注入。
var updateTagPattern = regexp.MustCompile(`^v?\d+(\.\d+)+(-[0-9A-Za-z.-]+)?$`)

// urlQueryRe 匹配 URL 的查询串（GitHub 资产直链的 sig/token 全在这里）。
var urlQueryRe = regexp.MustCompile(`(https?://[^\s"?#]+)\?[^\s"]*`)

// sanitizeURLText 抹掉 URL 查询串：签名直链不该进日志、更不该当错误正文怼到界面上。
func sanitizeURLText(s string) string {
	return urlQueryRe.ReplaceAllString(s, "$1")
}

// setUpdateProgress 推进进度；failed 同时释放重入标记放行下次更新。
func setUpdateProgress(phase, message, errText string) {
	updateStateMu.Lock()
	updateState.Phase = phase
	updateState.Message = message
	updateState.Error = errText
	if phase == phaseFailed {
		updateState.Running = false
		updateActive.Store(false)
	}
	updateState.UpdatedAt = time.Now()
	updateStateMu.Unlock()
}

// snapshotUpdateProgress 返回当前进度快照。
func snapshotUpdateProgress() updateProgress {
	updateStateMu.Lock()
	defer updateStateMu.Unlock()
	return updateState
}

// latestRelease 解析 GitHub API releases/latest 的最小响应。
type latestRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// AdminUpdateCheck GET /api/v1/admin/update/check —— 对比本地版本与 GitHub latest。
func (d *Deps) AdminUpdateCheck(c *gin.Context) {
	if d.Cfg == nil || !d.Cfg.Update.Enabled {
		util.OK(c, gin.H{"enabled": false, "current_version": PanelVersion})
		return
	}
	rel, tarballURL, shaURL, err := d.resolveLatest(c.Request.Context())
	if err != nil {
		util.Fail(c, 502, "查询更新源失败: "+err.Error())
		return
	}
	// 展示与实际下载同口径：配了 update.mirror 就是镜像直链，否则候选链首项（GitHub 直连）。
	if tarballURL != "" && rel.TagName != "" {
		if srcs := d.updateSources(rel.TagName); len(srcs) > 0 {
			tarballURL, shaURL = srcs[0].Tarball, srcs[0].Sha256
		}
	}
	available := rel.TagName != "" && rel.TagName != PanelVersion && tarballURL != ""
	util.OK(c, gin.H{
		"enabled":         true,
		"current_version": PanelVersion,
		"latest_version":  rel.TagName,
		"available":       available,
		"asset_url":       tarballURL,
		"sha256_url":      shaURL,
	})
}

// releaseAsset GitHub release 资产。
type releaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// releaseItem GitHub releases 列表项（仅取回滚/安装历史版本所需字段）。
type releaseItem struct {
	TagName     string         `json:"tag_name"`
	Name        string         `json:"name"`
	PublishedAt string         `json:"published_at"`
	Prerelease  bool           `json:"prerelease"`
	Draft       bool           `json:"draft"`
	Assets      []releaseAsset `json:"assets"`
}

// AdminUpdateReleases GET /api/v1/admin/update/releases —— 列出版本（供安装历史版本/回滚）。
// 返回最近 N 个 release 及当前架构资产是否可用；安装仍走 POST /admin/update/apply {version}，
// 与「应用更新」同一条下载→sha256→自检→原子替换→重启链路。
func (d *Deps) AdminUpdateReleases(c *gin.Context) {
	if d.Cfg == nil || !d.Cfg.Update.Enabled {
		util.OK(c, gin.H{"enabled": false, "current_version": PanelVersion, "releases": []any{}})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	items, err := d.fetchReleases(ctx)
	if err != nil {
		util.Fail(c, 502, "查询版本列表失败: "+err.Error())
		return
	}
	prefix := "https://github.com"
	if d.Cfg.Update.Mirror != "" {
		prefix = strings.TrimSuffix(d.Cfg.Update.Mirror, "/")
	}
	out := make([]gin.H, 0, len(items))
	for _, r := range items {
		if r.Draft || r.TagName == "" {
			continue
		}
		assetURL := assetURLForRelease(r, runtime.GOARCH, prefix)
		out = append(out, gin.H{
			"version":      r.TagName,
			"name":         r.Name,
			"published_at": r.PublishedAt,
			"prerelease":   r.Prerelease,
			"installable":  assetURL != "",
			"current":      r.TagName == PanelVersion,
			"asset_url":    assetURL,
		})
	}
	util.OK(c, gin.H{"enabled": true, "current_version": PanelVersion, "releases": out})
}

// assetURLForRelease 匹配当前架构的 release 资产名 xpanel-master-<tag>-linux-<arch>.tar.gz，
// 命中则返回（镜像前缀重写后的）下载直链，否则空串（视为该版本不可安装）。
func assetURLForRelease(r releaseItem, arch, githubPrefix string) string {
	want := fmt.Sprintf("xpanel-master-%s-linux-%s.tar.gz", r.TagName, arch)
	for _, a := range r.Assets {
		if a.Name == want {
			return strings.Replace(a.BrowserDownloadURL, "https://github.com", githubPrefix, 1)
		}
	}
	return ""
}

// fetchReleases 拉取 GitHub releases 列表（最近 30 个，足够回滚选取）。
func (d *Deps) fetchReleases(ctx context.Context) ([]releaseItem, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/releases?per_page=30", d.repo())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API 返回 %d", resp.StatusCode)
	}
	var items []releaseItem
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, err
	}
	return items, nil
}

// AdminUpdateApply POST /api/v1/admin/update/apply
// body: {"version": "vX.Y.Z"}（空 = 最新 release）。解析更新源后立即返回，剩余流程后台执行；
// 进度经 GET /admin/update/status 轮询。已有更新在执行时幂等返回当前进度（不报错，防手抖多击）。
func (d *Deps) AdminUpdateApply(c *gin.Context) {
	if d.Cfg == nil || !d.Cfg.Update.Enabled {
		util.BadRequest(c, "面板内更新已在配置文件中禁用")
		return
	}
	if !updateActive.CompareAndSwap(false, true) {
		util.OK(c, gin.H{"ok": true, "started": false, "progress": snapshotUpdateProgress(),
			"message": "更新正在进行中，请勿重复触发"})
		return
	}

	// 容器形态硬约束：仅当程序运行在挂载形态（/app/master 可写位置存在）时允许自更新
	appRoot := "/app"
	masterPath := filepath.Join(appRoot, "master")
	if _, err := os.Stat(masterPath); err != nil {
		updateActive.Store(false)
		util.BadRequest(c, "面板内更新仅在 compose 容器部署下可用（未检测到挂载形态）")
		return
	}

	var req struct {
		Version string `json:"version"`
	}
	_ = c.ShouldBindJSON(&req)

	updateStateMu.Lock()
	updateState = updateProgress{
		Running:   true,
		Phase:     phaseChecking,
		From:      PanelVersion,
		Target:    req.Version,
		Message:   "正在解析更新源...",
		StartedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	updateStateMu.Unlock()

	// 1. 解析目标版本：指定版本只做格式校验，空走 releases/latest（同步完成，耗时≤10s）。
	//    具体下载源在 runUpdate 里按候选链逐个尝试。
	var (
		rel *latestRelease
		err error
	)
	version := strings.TrimSpace(req.Version)
	if version != "" {
		if !updateTagPattern.MatchString(version) {
			updateActive.Store(false)
			util.BadRequest(c, "版本号格式不正确")
			return
		}
	} else {
		var assetURL string
		rel, assetURL, _, err = d.resolveLatest(c.Request.Context())
		if err != nil {
			setUpdateProgress(phaseFailed, "查询更新源失败", err.Error())
			util.Fail(c, 502, "查询更新源失败: "+err.Error())
			return
		}
		if assetURL == "" {
			msg := "更新源未提供当前架构（" + runtime.GOARCH + "）的安装包"
			setUpdateProgress(phaseFailed, msg, "")
			util.BadRequest(c, msg)
			return
		}
		version = rel.TagName
	}
	if version == "" {
		updateActive.Store(false)
		util.BadRequest(c, "无法确定目标版本")
		return
	}
	if version == PanelVersion && strings.TrimSpace(req.Version) == "" {
		// 仅「latest 解析后与当前一致」短路；显式指定同版本视为重装修复，照旧执行。
		updateStateMu.Lock()
		updateState = updateProgress{Phase: "success", Message: fmt.Sprintf("已是最新版本 %s，无需更新", PanelVersion)}
		updateStateMu.Unlock()
		updateActive.Store(false)
		util.OK(c, gin.H{"ok": true, "started": false, "version": PanelVersion,
			"progress": snapshotUpdateProgress(), "message": "已是最新版本，无需更新"})
		return
	}

	updateStateMu.Lock()
	updateState.Target = version
	updateStateMu.Unlock()

	// 2. 下载/校验/替换/重启转后台执行；上下文刻意脱离请求（10 分钟总预算，对齐 agent
	//    下载超时）：响应返回、前端断开、反代掐断都不再影响更新本身。
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	go func() {
		defer cancel()
		d.runUpdate(ctx, appRoot, masterPath, d.updateSources(version), version)
	}()

	log.Printf("update: 已触发后台更新 %s → %s（%s）", PanelVersion, version, runtime.GOARCH)
	util.OK(c, gin.H{"ok": true, "started": true, "version": version,
		"progress": snapshotUpdateProgress(), "message": "更新已开始，正在后台执行（下载 → 校验 → 替换 → 容器重启）"})
}

// AdminUpdateStatus GET /api/v1/admin/update/status —— 自更新进度轮询端点。
// 进程随重启退出、内存状态清零：重启后的成败由前端比对 current_version 与目标版本判定
// （一致=成功；不一致=新版本启动失败已被 entrypoint 回滚）。
func (d *Deps) AdminUpdateStatus(c *gin.Context) {
	util.OK(c, gin.H{
		"enabled":         d.Cfg != nil && d.Cfg.Update.Enabled,
		"current_version": PanelVersion,
		"progress":        snapshotUpdateProgress(),
	})
}

// runUpdate 后台执行 下载 → sha256 校验 → 解压/自检 → 备份/替换 → SIGTERM 重启，
// 阶段推进经 setUpdateProgress 暴露（对齐 agent runUpgrade 的 report 模式）。
func (d *Deps) runUpdate(ctx context.Context, appRoot, masterPath string, sources []updateSource, version string) {
	// staging 必须与替换目标同挂载点（/app = 宿主安装目录整挂）才能原子 rename：
	// /app/data 是独立 bind mount（./data:/app/data），staging 放那里跨挂载 rename 必报
	// EXDEV（invalid cross-device link），自更新会卡死在替换一步（2026-08-31 实机复现路径）。
	stage, err := os.MkdirTemp(appRoot, ".update-staging-")
	if err != nil {
		setUpdateProgress(phaseFailed, "创建暂存目录失败", err.Error())
		return
	}
	defer os.RemoveAll(stage)

	// 下载（独立协程每秒刷新已下载字节数——「卡在下载」从此肉眼可见；
	// 已推进到校验/换源阶段时不再覆盖进度文案）
	tarball := filepath.Join(stage, "package.tar.gz")
	downloaded := new(atomic.Int64)
	stopProgress := make(chan struct{})
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-stopProgress:
				return
			case <-t.C:
				if snapshotUpdateProgress().Phase != phaseDownloading {
					continue
				}
				setUpdateProgress(phaseDownloading,
					fmt.Sprintf("正在下载更新包...（已下载 %.1f MB）", float64(downloaded.Load())/1024/1024), "")
			}
		}
	}()

	// 按候选链逐个源「下载 + 同源校验」：某个源握手抖动或没有该资产就换下一个，
	// 全部源都失败才判更新失败（旧实现单源单次，一次抖动整单作废）。
	var lastErr error
	for i, src := range sources {
		if err := ctx.Err(); err != nil {
			lastErr = err
			break
		}
		msg := "正在下载更新包..."
		if i > 0 {
			msg = fmt.Sprintf("切换备用下载源（%d/%d）...", i+1, len(sources))
		}
		setUpdateProgress(phaseDownloading, msg, "")
		log.Printf("update: 下载 %s（源 %d/%d）", src.Base, i+1, len(sources))

		downloaded.Store(0)
		if err := downloadFile(ctx, src.Tarball, tarball, downloaded); err != nil {
			lastErr = err
			log.Printf("update: 下载失败（源 %s）: %v", src.Base, err)
			continue
		}

		setUpdateProgress(phaseVerifying,
			fmt.Sprintf("下载完成（%.1f MB），正在校验 sha256...", float64(downloaded.Load())/1024/1024), "")
		hash, err := sha256File(tarball)
		if err != nil {
			close(stopProgress)
			setUpdateProgress(phaseFailed, "计算校验失败", err.Error())
			return
		}
		if err := d.verifyChecksum(ctx, src.Sha256, tarball, hash); err != nil {
			lastErr = err
			log.Printf("update: 校验失败（源 %s）: %v", src.Base, err)
			continue
		}
		lastErr = nil
		log.Printf("update: 已从 %s 下载并校验更新包（%.1f MB）", src.Base, float64(downloaded.Load())/1024/1024)
		break
	}
	close(stopProgress)
	if lastErr != nil || len(sources) == 0 {
		if lastErr == nil {
			lastErr = fmt.Errorf("没有可用的下载源")
		}
		setUpdateProgress(phaseFailed, "下载更新包失败（已尝试全部下载源）", lastErr.Error())
		return
	}

	// 解压（白名单提取 master + web/dist，防路径穿越）
	setUpdateProgress(phaseVerifying, "正在解压更新包...", "")
	unpacked := filepath.Join(stage, "unpacked")
	if err := extractTarGz(tarball, unpacked); err != nil {
		setUpdateProgress(phaseFailed, "解压更新包失败", err.Error())
		return
	}
	newBin := filepath.Join(unpacked, "master", "master")
	newDist := filepath.Join(unpacked, "web", "dist")
	if _, err := os.Stat(newBin); err != nil {
		setUpdateProgress(phaseFailed, "更新包缺少 master/master（包结构不符）", "")
		return
	}
	if _, err := os.Stat(newDist); err != nil {
		setUpdateProgress(phaseFailed, "更新包缺少 web/dist（包结构不符）", "")
		return
	}

	// 冒烟：新二进制 -self-test（不连库，无副作用；挡架构错误/损坏/配置不兼容）
	setUpdateProgress(phaseVerifying, "正在自检新版本二进制（-self-test）...", "")
	out, err := exec.CommandContext(ctx, newBin, "-self-test", "-config", updateCfgCfg).CombinedOutput()
	if err != nil {
		setUpdateProgress(phaseFailed, "新版本二进制自检未通过，拒绝替换",
			fmt.Sprintf("%v / %s", err, strings.TrimSpace(string(out))))
		return
	}
	log.Printf("update: 自检通过: %s", strings.TrimSpace(string(out)))

	// 备份当前版本 + 落待确认标记（供 entrypoint 回滚判定）
	setUpdateProgress(phaseReplacing, "正在备份当前版本...", "")
	cur, err := os.ReadFile(masterPath)
	if err != nil {
		setUpdateProgress(phaseFailed, "读取当前版本失败", err.Error())
		return
	}
	if err := os.WriteFile(masterPath+".prev", cur, 0o755); err != nil {
		setUpdateProgress(phaseFailed, "备份当前版本失败", err.Error())
		return
	}
	webDist := filepath.Join(appRoot, "web", "dist")
	if err := copyDir(webDist, webDist+".prev"); err != nil {
		setUpdateProgress(phaseFailed, "备份前端失败", err.Error())
		return
	}
	if err := os.WriteFile(filepath.Join(appRoot, ".update-pending"), []byte(version), 0o644); err != nil {
		setUpdateProgress(phaseFailed, "写入更新标记失败", err.Error())
		return
	}

	// 原子替换（staging 与 /app 同文件系统，rename 原子）
	setUpdateProgress(phaseReplacing, "正在替换 master 与前端产物...", "")
	if err := os.Rename(newBin, masterPath); err != nil {
		setUpdateProgress(phaseFailed, "替换 master 失败", err.Error())
		return
	}
	if err := os.RemoveAll(webDist); err != nil {
		setUpdateProgress(phaseFailed, "清理旧前端失败", err.Error())
		return
	}
	if err := os.Rename(newDist, webDist); err != nil {
		setUpdateProgress(phaseFailed, "替换前端失败", err.Error())
		return
	}

	// 触发优雅退出：SIGTERM → 既有 signal handler 关停全链路 → 进程退出 0 →
	// Docker 检测到容器停止（非人工 stop）→ unless-stopped 拉起新版本。
	// 稍候 2s 再发信号：给前端轮询留出观察到 restarting 阶段的时间窗。
	setUpdateProgress(phaseRestarting,
		fmt.Sprintf("已替换为 %s，容器即将重启加载新版本（面板将短暂离线）...", version), "")
	log.Printf("update: 已替换为 %s（%s），2s 后触发重启", version, runtime.GOARCH)
	time.Sleep(2 * time.Second)
	p, err := os.FindProcess(os.Getpid())
	if err == nil {
		err = p.Signal(syscall.SIGTERM)
	}
	if err != nil {
		// 进程没死成：释放重入放行重试，状态留 failed 提示用户
		setUpdateProgress(phaseFailed, "触发容器重启失败，可重试应用更新", err.Error())
		return
	}
	// 信号已发出，优雅退出将终结本进程；此处直接返回（updateActive 保持占用至进程消亡）
}

// resolveLatest 查 GitHub releases/latest 并匹配当前架构资产（与 install.sh 同规则）。
func (d *Deps) resolveLatest(ctx context.Context) (*latestRelease, string, string, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", d.repo())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, "", "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", "", fmt.Errorf("GitHub API 返回 %d", resp.StatusCode)
	}
	var rel latestRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, "", "", err
	}
	want := fmt.Sprintf("xpanel-master-%s-linux-%s.tar.gz", rel.TagName, runtime.GOARCH)
	tarballURL, shaURL := "", ""
	for _, a := range rel.Assets {
		if a.Name == want {
			// 只返回 github.com 直链：镜像前缀统一由 updateSources 派生，避免两处各算一套。
			tarballURL = a.BrowserDownloadURL
			shaURL = strings.TrimSuffix(tarballURL, ".tar.gz") + ".tar.gz.sha256"
			break
		}
	}
	return &rel, tarballURL, shaURL, nil
}

// updateSource 一个下载候选源：压缩包与其 .sha256 同源取齐，避免跨源校验错配。
type updateSource struct {
	Base    string
	Tarball string
	Sha256  string
}

// updateSources 生成下载候选链：update.mirror（配置了则置顶）+ 内置候选，去空去重。
// 只改写 https://github.com 前缀（与 install.sh 的「优先镜像」语义一致）。
func (d *Deps) updateSources(version string) []updateSource {
	canonical := fmt.Sprintf("https://github.com/%s/releases/download/%s/xpanel-master-%s-linux-%s.tar.gz",
		d.repo(), version, version, runtime.GOARCH)
	bases := make([]string, 0, len(updateBuiltinMirrors)+1)
	if d.Cfg != nil {
		if m := strings.TrimSpace(d.Cfg.Update.Mirror); m != "" {
			bases = append(bases, m)
		}
	}
	bases = append(bases, updateBuiltinMirrors...)

	seen := make(map[string]bool, len(bases))
	out := make([]updateSource, 0, len(bases))
	for _, base := range bases {
		base = strings.TrimSuffix(strings.TrimSpace(base), "/")
		if base == "" || seen[base] {
			continue
		}
		seen[base] = true
		tarball := strings.Replace(canonical, "https://github.com", base, 1)
		out = append(out, updateSource{Base: base, Tarball: tarball, Sha256: tarball + ".sha256"})
	}
	return out
}

func (d *Deps) repo() string {
	if d.Cfg != nil && d.Cfg.Update.Repo != "" {
		return d.Cfg.Update.Repo
	}
	return "acdc-awa/xpanel"
}

// verifyChecksum 用 release 的 .sha256 强制校验（与 install.sh 相同信任模型）。
func (d *Deps) verifyChecksum(ctx context.Context, shaURL, tarball, actual string) error {
	if shaURL == "" {
		return fmt.Errorf("缺少校验和地址")
	}
	var buf strings.Builder
	if err := fetchWithRetry(ctx, "下载校验和", func() error {
		buf.Reset() // 重试时清空上一次的半截内容
		return downloadTo(ctx, shaURL, &buf)
	}); err != nil {
		return err
	}
	fields := strings.Fields(buf.String())
	if len(fields) == 0 {
		return fmt.Errorf("校验和文件为空")
	}
	if !strings.EqualFold(fields[0], actual) {
		return fmt.Errorf("期望 %s 实际 %s", fields[0], actual)
	}
	return nil
}

// downloadFile 下载 URL 到本地文件，并累加已下载字节数到 counter（进度展示用）。
func downloadFile(ctx context.Context, url, path string, counter *atomic.Int64) error {
	return fetchWithRetry(ctx, "下载更新包", func() error {
		counter.Store(0)
		f, err := os.Create(path)
		if err != nil {
			return err
		}
		defer f.Close()
		return downloadTo(ctx, url, io.MultiWriter(f, &counterWriter{n: counter}))
	})
}

// fetchWithRetry 对一次下载动作做同源重试。连接/握手期的抖动重试同一源通常就能过
// （线上每次发版第一次尝试都会撞上一次 10s TLS 握手超时）；内容性错误（4xx、ctx 结束）
// 不重试，交给调用方换源或直接失败。
func fetchWithRetry(ctx context.Context, label string, fn func() error) error {
	var lastErr error
	for attempt := 1; attempt <= updateDownloadAttempts; attempt++ {
		if attempt > 1 {
			wait := updateRetryBackoff[0]
			if attempt-2 < len(updateRetryBackoff) {
				wait = updateRetryBackoff[attempt-2]
			}
			log.Printf("update: %s 第 %d/%d 次失败，%s 后重试: %v",
				label, attempt-1, updateDownloadAttempts, wait, lastErr)
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
		lastErr = fn()
		if lastErr == nil {
			return nil
		}
		if !updateRetryable(lastErr) {
			return lastErr
		}
	}
	return lastErr
}

// updateHTTPError 下载源返回了非 200 状态：4xx 多为该源没有这个资产，换源比重试有效。
type updateHTTPError struct {
	URL    string
	Status int
}

func (e *updateHTTPError) Error() string {
	return fmt.Sprintf("下载 %s 返回 %d", sanitizeURLText(e.URL), e.Status)
}

// updateRetryable 判断一次下载失败值不值得重试同一源。
func updateRetryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var httpErr *updateHTTPError
	if errors.As(err, &httpErr) {
		return httpErr.Status >= 500 ||
			httpErr.Status == http.StatusRequestTimeout ||
			httpErr.Status == http.StatusTooManyRequests
	}
	return true // 拨号/TLS/读超时等网络性错误
}

// counterWriter 把写入字节数累加进原子计数器（下载进度协程每秒读取刷新）。
type counterWriter struct{ n *atomic.Int64 }

func (w *counterWriter) Write(p []byte) (int, error) {
	w.n.Add(int64(len(p)))
	return len(p), nil
}

// downloadTo 下载 URL 内容写入 writer；用 updateClient（独立超时配比），
// 整体生命周期由调用方 ctx 约束（更新包走 runUpdate 的 10 分钟总预算）。
// 错误文本统一脱敏：GitHub 302 之后的直链带 sig/token，不该外泄。
func downloadTo(ctx context.Context, url string, w io.Writer) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := updateClient.Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return errors.New(sanitizeURLText(err.Error()))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return &updateHTTPError{URL: url, Status: resp.StatusCode}
	}
	if _, err := io.Copy(w, resp.Body); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return errors.New(sanitizeURLText(err.Error()))
	}
	return nil
}

// sha256File 计算文件 sha256（hex）。
func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// extractTarGz 解压 gzip tar 到 dest，仅提取 master/ 与 web/dist/，白名单防路径穿越。
func extractTarGz(tarball, dest string) error {
	f, err := os.Open(tarball)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name := filepath.Clean(hdr.Name)
		if name == "." || name == "" {
			continue
		}
		// 白名单：仅 master/ 与 web/dist/（跳过 README/install.sh 等）
		if name != "master/master" && name != "web/dist" && !strings.HasPrefix(name, "web/dist/") {
			continue
		}
		if strings.Contains(name, "..") {
			return fmt.Errorf("无效的路径 %q", hdr.Name)
		}
		target := filepath.Join(dest, name)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode)&0o777)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		}
	}
	return nil
}

// copyDir 递归复制目录（备份用）。
func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}
