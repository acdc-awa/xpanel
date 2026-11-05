package api

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/acdc-awa/xpanel/internal/pkg/util"
)

// 本文件：面板侧可选的 Xray 内核 / Agent 版本目录（供「自由升降级」下拉框使用）。
//
// 版本来源是 GitHub REST 的 releases 列表，而不是 /releases/latest：
//   - XTLS 的发布全部标记为 prerelease，latest 会退回 v26.3.27；
//   - 列表接口一次响应就带 tag 与 assets，可以只保留「发布包齐备」的版本。
//
// 历史坑：原先读 releases.atom 免限流，但 atom 会把**只有 tag、没有发布**的提交一并列出
// （XPanel-Node 实际只有 3 个 release，atom 却给出 10 条 tag），选中这种版本下载必然 404。
const (
	defaultXrayRepo = "XTLS/Xray-core"
	xrayVersionTTL  = 15 * time.Minute
	xrayVersionMax  = 30
)

// githubAPIBase GitHub REST 基址（单测替换为 httptest 地址）。
var githubAPIBase = "https://api.github.com"

type xrayVersionCacheEntry struct {
	sync.RWMutex
	versions  []string
	checkedAt time.Time
}

var globalXrayVersionCache xrayVersionCacheEntry

type xrayAtomFeed struct {
	Entries []struct {
		Links []struct {
			Href string `xml:"href,attr"`
		} `xml:"link"`
	} `xml:"entry"`
}

// FetchXrayVersions 取最近可安装的内核版本（含 prerelease，按新→旧）。
func FetchXrayVersions(ctx context.Context, mirror string) ([]string, error) {
	return FetchRepoVersions(ctx, mirror, defaultXrayRepo)
}

// FetchRepoVersions 取仓库最近的可安装版本（新→旧）。
//
// 只列「发布包齐备」的 release：纯 tag（无发布包）、草稿、资产不齐的一律剔除，
// 否则用户选中一个下载 404 的版本。REST 不可用（匿名限流 / 内网只能走镜像）时
// 才回退 releases.atom——atom 分不出 tag 与 release，属降级路径，留日志痕迹。
func FetchRepoVersions(ctx context.Context, mirror, repo string) ([]string, error) {
	versions, err := fetchRepoVersionsREST(ctx, repo)
	if err == nil {
		return versions, nil
	}
	log.Printf("[versions] %s 取 REST 版本列表失败（%v），回退 releases.atom", repo, err)
	return fetchRepoVersionsAtom(ctx, mirror, repo)
}

// repoAssetsComplete 该 release 的资产是否够节点安装，口径与 version_probe.go 一致
// （那边判「最新版本能不能升」，这边判「历史版本能不能选」，同一个取用形态）。
func repoAssetsComplete(repo string, assets []releaseAsset) bool {
	set := make(map[string]bool, len(assets))
	for _, a := range assets {
		set[a.Name] = true
	}
	switch repo {
	case defaultAgentRepo:
		return agentAssetsComplete(set)
	case defaultXrayRepo:
		return xrayAssetsComplete(set)
	}
	return len(set) > 0
}

// fetchRepoVersionsREST 读 GitHub releases 列表并过滤出可安装版本。
func fetchRepoVersionsREST(ctx context.Context, repo string) ([]string, error) {
	apiURL := fmt.Sprintf("%s/repos/%s/releases?per_page=%d", strings.TrimSuffix(githubAPIBase, "/"), repo, xrayVersionMax)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var items []releaseItem
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&items); err != nil {
		return nil, fmt.Errorf("解析 releases 列表失败: %w", err)
	}

	out := make([]string, 0, len(items))
	seen := make(map[string]bool, len(items))
	for _, r := range items {
		tag := strings.TrimSpace(r.TagName)
		if r.Draft || tag == "" || seen[tag] || !repoAssetsComplete(repo, r.Assets) {
			continue
		}
		seen[tag] = true
		out = append(out, tag)
		if len(out) >= xrayVersionMax {
			break
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("没有可安装的发布版本（资产不齐或全部为草稿）")
	}
	return out, nil
}

// fetchRepoVersionsAtom 降级路径：从 releases.atom 取最近版本（含纯 tag）。
func fetchRepoVersionsAtom(ctx context.Context, mirror, repo string) ([]string, error) {
	base := "https://github.com"
	if strings.TrimSpace(mirror) != "" {
		base = strings.TrimSuffix(strings.TrimSpace(mirror), "/")
	}
	url := base + "/" + repo + "/releases.atom"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	var feed xrayAtomFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("解析 releases.atom 失败: %w", err)
	}

	seen := make(map[string]bool)
	out := make([]string, 0, len(feed.Entries))
	const marker = "/releases/tag/"
	for _, e := range feed.Entries {
		for _, l := range e.Links {
			i := strings.LastIndex(l.Href, marker)
			if i < 0 {
				continue
			}
			tag := strings.TrimSpace(l.Href[i+len(marker):])
			if tag == "" || seen[tag] {
				continue
			}
			seen[tag] = true
			out = append(out, tag)
			if len(out) >= xrayVersionMax {
				break
			}
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("releases.atom 中没有版本信息")
	}
	return out, nil
}

// GetCachedXrayVersions 返回缓存的版本列表；过期或 force 时重新拉取，失败时降级用旧缓存。
func (d *Deps) GetCachedXrayVersions(ctx context.Context, force bool) ([]string, time.Time, error) {
	globalXrayVersionCache.RLock()
	versions := append([]string(nil), globalXrayVersionCache.versions...)
	checked := globalXrayVersionCache.checkedAt
	globalXrayVersionCache.RUnlock()

	if !force && len(versions) > 0 && time.Since(checked) < xrayVersionTTL {
		return versions, checked, nil
	}

	mirror := ""
	if d.Cfg != nil {
		mirror = d.Cfg.Update.Mirror
	}
	got, err := FetchXrayVersions(ctx, mirror)
	if err != nil {
		if len(versions) > 0 {
			return versions, checked, nil
		}
		return nil, time.Time{}, err
	}

	now := time.Now()
	globalXrayVersionCache.Lock()
	globalXrayVersionCache.versions = got
	globalXrayVersionCache.checkedAt = now
	globalXrayVersionCache.Unlock()
	return got, now, nil
}

// AdminGetXrayVersions GET /api/v1/admin/servers/xray-versions?refresh=1
// 返回可选的内核版本（新→旧）、最新版本与检测时间。拉取失败但有缓存时返回旧缓存。
func (d *Deps) AdminGetXrayVersions(c *gin.Context) {
	force := c.Query("refresh") == "1" || c.Query("refresh") == "true"
	versions, checkedAt, err := d.GetCachedXrayVersions(c.Request.Context(), force)
	if err != nil {
		util.Fail(c, 502, "获取 Xray 版本列表失败: "+err.Error())
		return
	}
	util.OK(c, gin.H{
		"repo":           defaultXrayRepo,
		"latest_version": versions[0],
		"versions":       versions,
		"checked_at":     checkedAt.Format(time.RFC3339),
	})
}
