package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"
)

// versionProbeInterval 版本探测间隔。
//
// 探测放在后台跑，请求路径只读内存缓存：这样既能用一次完整的「release 资产是否齐备」校验
// 换掉请求期的猜测，又不会把 api.github.com 的匿名限流（60 次/小时）暴露给用户操作。
// 两个仓库 × 每 10 分钟 = 12 次/小时，余量充足。
const versionProbeInterval = 10 * time.Minute

// releaseProbe 一次版本探测的结果。
//
// 只有 Verified 为真才代表「这个版本真的可以下载」：仓库里可能有 tag 而没有 release，
// 也可能 release 已创建但资产还在上传（两个仓库的 release.yml 都不走 draft，存在这个窗口）。
type releaseProbe struct {
	Version   string
	Verified  bool
	CheckedAt time.Time
	Err       string
}

var (
	releaseProbeMu    sync.RWMutex
	panelReleaseProbe releaseProbe
	agentReleaseProbe releaseProbe
	xrayReleaseProbe  releaseProbe
)

// panelProbeSnapshot 返回面板最新版本探测结果（只读，绝不触发网络）。
func panelProbeSnapshot() releaseProbe {
	releaseProbeMu.RLock()
	defer releaseProbeMu.RUnlock()
	return panelReleaseProbe
}

// agentProbeSnapshot 返回 Agent 最新版本探测结果（只读，绝不触发网络）。
func agentProbeSnapshot() releaseProbe {
	releaseProbeMu.RLock()
	defer releaseProbeMu.RUnlock()
	return agentReleaseProbe
}

// xrayProbeSnapshot 返回 Xray 内核最新版本探测结果（只读，绝不触发网络）。
func xrayProbeSnapshot() releaseProbe {
	releaseProbeMu.RLock()
	defer releaseProbeMu.RUnlock()
	return xrayReleaseProbe
}

// StartVersionProbe 启动版本探测后台任务：启动即跑一轮，之后每 versionProbeInterval 刷新一次。
func StartVersionProbe(ctx context.Context, d *Deps) {
	go func() {
		d.probeReleases(ctx)
		ticker := time.NewTicker(versionProbeInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				d.probeReleases(ctx)
			}
		}
	}()
}

// probeReleases 刷新面板与 Agent 的最新版本缓存。任何一路失败都不影响另一路，
// 且失败时保留上一次成功结果（版本发布本身不会消失，沿用旧值比清空更接近事实）。
func (d *Deps) probeReleases(ctx context.Context) {
	mirror := ""
	if d.Cfg != nil {
		mirror = d.Cfg.Update.Mirror
	}

	// 面板：自更新关闭时不探测（此时面板不提供更新入口，提示也没有去处）。
	if d.Cfg == nil || d.Cfg.Update.Enabled {
		tag, assets, err := fetchLatestReleaseAssets(ctx, mirror, d.repo())
		storeReleaseProbe(&panelReleaseProbe, tag, err == nil && panelAssetsComplete(tag, assets), err)
	}

	tag, assets, err := fetchLatestReleaseAssets(ctx, mirror, defaultAgentRepo)
	storeReleaseProbe(&agentReleaseProbe, tag, err == nil && agentAssetsComplete(assets), err)

	// Xray 内核：面板不下载资产（由节点升级时拉取），但仍按节点的取用形态校验，
	// 避免在发布包还没传完时提示一个升不上去的版本。
	tag, assets, err = fetchLatestReleaseAssets(ctx, mirror, defaultXrayRepo)
	storeReleaseProbe(&xrayReleaseProbe, tag, err == nil && xrayAssetsComplete(assets), err)
}

func storeReleaseProbe(target *releaseProbe, version string, verified bool, err error) {
	releaseProbeMu.Lock()
	defer releaseProbeMu.Unlock()
	target.CheckedAt = time.Now()
	if err != nil {
		// 同样的错误只记一次：内网/无出网部署下每 10 分钟一条会刷满日志。
		if target.Err != err.Error() {
			log.Printf("[version-probe] %v", err)
		}
		target.Err = err.Error()
		return
	}
	target.Version = version
	target.Verified = verified
	target.Err = ""
}

// fetchLatestReleaseAssets 读 GitHub releases/latest，返回 tag 与资产名集合。
//
// 用 REST 而不是 302 重定向：一次响应同时拿到 tag 与 assets，能在同一时刻判断资产是否齐备；
// 重定向只给 tag，还得再打一次 HEAD 才知道能不能下载。
func fetchLatestReleaseAssets(ctx context.Context, mirror, repo string) (string, map[string]bool, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("探测 %s 最新版本失败: %w", repo, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("探测 %s 最新版本失败: GitHub API 返回 %d", repo, resp.StatusCode)
	}

	var rel latestRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", nil, fmt.Errorf("解析 %s 最新版本失败: %w", repo, err)
	}
	tag := strings.TrimSpace(rel.TagName)
	if tag == "" {
		return "", nil, fmt.Errorf("探测 %s 最新版本失败: tag 为空", repo)
	}
	assets := make(map[string]bool, len(rel.Assets))
	for _, a := range rel.Assets {
		assets[a.Name] = true
	}
	return tag, assets, nil
}

// panelAssetsComplete 面板更新包必须齐备：本机架构的压缩包 + 对应校验和。
func panelAssetsComplete(tag string, assets map[string]bool) bool {
	tarball := fmt.Sprintf("xpanel-master-%s-linux-%s.tar.gz", tag, runtime.GOARCH)
	return assets[tarball] && assets[tarball+".sha256"]
}

// agentAssetsComplete Agent 资产必须齐备：checksums.txt + 至少一个架构的二进制。
// 不按面板本机架构校验——节点可能是另一种架构，面板所在机器无法代表节点。
func agentAssetsComplete(assets map[string]bool) bool {
	if !assets["checksums.txt"] {
		return false
	}
	for name := range assets {
		if strings.HasPrefix(name, "xray-agent-linux-") {
			return true
		}
	}
	return false
}

// xrayAssetsComplete Xray 内核包必须齐备：任一架构的 zip + 同名 .dgst。
// 与 agent 升级时的取用形态一致（agent/upgrade/xray.go 要求 zip 与 .dgst 成对，
// 缺 .dgst 会被拒绝升级），所以只有成对存在才算这个版本可用。
func xrayAssetsComplete(assets map[string]bool) bool {
	for _, name := range []string{"Xray-linux-64.zip", "Xray-linux-arm64-v8a.zip"} {
		if assets[name] && assets[name+".dgst"] {
			return true
		}
	}
	return false
}
