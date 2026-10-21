package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/acdc-awa/xpanel/internal/pkg/util"
)

const (
	defaultAgentRepo = "acdc-awa/XPanel-Node"
	agentVersionTTL  = 15 * time.Minute
)

// agentVersionCacheEntry Agent 最新版本内存缓存
type agentVersionCacheEntry struct {
	sync.RWMutex
	latestVersion string
	checkedAt     time.Time
}

var globalAgentVersionCache agentVersionCacheEntry

// NormAgentVersion 归一化版本串：去掉 v 前缀，截断第一个 - 之前的部分。
func NormAgentVersion(s string) string {
	s = strings.TrimSpace(strings.TrimPrefix(s, "v"))
	if i := strings.Index(s, "-"); i >= 0 {
		s = s[:i]
	}
	if s == "" {
		return "0"
	}
	for _, seg := range strings.Split(s, ".") {
		if seg == "" {
			return "0"
		}
		if _, err := strconv.Atoi(seg); err != nil {
			return "0"
		}
	}
	return s
}

// CompareAgentVersion 语义化比较版本：-1 a<b / 0 相等 / 1 a>b。
func CompareAgentVersion(a, b string) int {
	na := NormAgentVersion(a)
	nb := NormAgentVersion(b)
	as := strings.Split(na, ".")
	bs := strings.Split(nb, ".")
	n := len(as)
	if len(bs) > n {
		n = len(bs)
	}
	for i := 0; i < n; i++ {
		ai, bi := 0, 0
		if i < len(as) {
			ai, _ = strconv.Atoi(as[i])
		}
		if i < len(bs) {
			bi, _ = strconv.Atoi(bs[i])
		}
		if ai < bi {
			return -1
		}
		if ai > bi {
			return 1
		}
	}
	return 0
}

// FetchLatestAgentVersion 从 GitHub Releases 获取最新 release tag（通过 302 重定向 Location 解析，免 API 限频）
func FetchLatestAgentVersion(ctx context.Context, mirror string) (string, error) {
	base := "https://github.com"
	if mirror != "" {
		base = strings.TrimSuffix(mirror, "/")
	}
	url := fmt.Sprintf("%s/%s/releases/latest", base, defaultAgentRepo)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	cli := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := cli.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	loc := resp.Header.Get("Location")
	if loc == "" {
		return "", fmt.Errorf("未找到最新 release（HTTP %d 无重定向）", resp.StatusCode)
	}

	const marker = "/releases/tag/"
	i := strings.LastIndex(loc, marker)
	if i < 0 {
		return "", fmt.Errorf("无法从重定向解析版本: %s", loc)
	}
	tag := strings.TrimSpace(loc[i+len(marker):])
	if tag == "" {
		return "", fmt.Errorf("解析 tag 为空: %s", loc)
	}
	return tag, nil
}

// GetCachedAgentLatestVersion 获取缓存的最新 Agent 版本，过期或无缓存时触发拉取
func (d *Deps) GetCachedAgentLatestVersion(ctx context.Context, force bool) (string, time.Time, error) {
	globalAgentVersionCache.RLock()
	ver := globalAgentVersionCache.latestVersion
	checked := globalAgentVersionCache.checkedAt
	globalAgentVersionCache.RUnlock()

	if !force && ver != "" && time.Since(checked) < agentVersionTTL {
		return ver, checked, nil
	}

	mirror := ""
	if d.Cfg != nil && d.Cfg.Update.Mirror != "" {
		mirror = d.Cfg.Update.Mirror
	}

	latest, err := FetchLatestAgentVersion(ctx, mirror)
	if err != nil {
		// 拉取失败但有旧缓存时优雅降级返回旧缓存
		if ver != "" {
			return ver, checked, nil
		}
		return "", time.Time{}, err
	}

	now := time.Now()
	globalAgentVersionCache.Lock()
	globalAgentVersionCache.latestVersion = latest
	globalAgentVersionCache.checkedAt = now
	globalAgentVersionCache.Unlock()

	return latest, now, nil
}

// AdminGetAgentVersion GET /api/v1/admin/servers/agent-version
// 返回当前官方最新发布的 Agent 版本及检测时间
func (d *Deps) AdminGetAgentVersion(c *gin.Context) {
	force := c.Query("refresh") == "1" || c.Query("refresh") == "true"
	ver, checkedAt, err := d.GetCachedAgentLatestVersion(c.Request.Context(), force)
	if err != nil {
		util.Fail(c, 502, "获取 Agent 最新版本失败: "+err.Error())
		return
	}
	util.OK(c, gin.H{
		"latest_version": ver,
		"repo":           defaultAgentRepo,
		"checked_at":     checkedAt.Format(time.RFC3339),
	})
}

// ---- Agent 版本列表（面板内自由升级/降级共用，与 Xray 版本目录同源） ----

type agentVersionsCacheEntry struct {
	sync.RWMutex
	versions  []string
	checkedAt time.Time
}

var globalAgentVersionsCache agentVersionsCacheEntry

// GetCachedAgentVersions 返回 Agent 可选版本（新→旧）；过期或 force 时重新拉取，
// 拉取失败时降级返回旧缓存。
func (d *Deps) GetCachedAgentVersions(ctx context.Context, force bool) ([]string, time.Time, error) {
	globalAgentVersionsCache.RLock()
	versions := append([]string(nil), globalAgentVersionsCache.versions...)
	checked := globalAgentVersionsCache.checkedAt
	globalAgentVersionsCache.RUnlock()

	if !force && len(versions) > 0 && time.Since(checked) < agentVersionTTL {
		return versions, checked, nil
	}

	mirror := ""
	if d.Cfg != nil {
		mirror = d.Cfg.Update.Mirror
	}
	got, err := FetchRepoVersions(ctx, mirror, defaultAgentRepo)
	if err != nil {
		if len(versions) > 0 {
			return versions, checked, nil
		}
		return nil, time.Time{}, err
	}

	now := time.Now()
	globalAgentVersionsCache.Lock()
	globalAgentVersionsCache.versions = got
	globalAgentVersionsCache.checkedAt = now
	globalAgentVersionsCache.Unlock()
	return got, now, nil
}

// AdminGetAgentVersions GET /api/v1/admin/servers/agent-versions?refresh=1
// 返回最近发布的 Agent 版本列表（含 prerelease），供面板的版本选择器使用。
func (d *Deps) AdminGetAgentVersions(c *gin.Context) {
	force := c.Query("refresh") == "1" || c.Query("refresh") == "true"
	versions, checkedAt, err := d.GetCachedAgentVersions(c.Request.Context(), force)
	if err != nil {
		util.Fail(c, 502, "获取 Agent 版本列表失败: "+err.Error())
		return
	}
	util.OK(c, gin.H{
		"repo":           defaultAgentRepo,
		"latest_version": versions[0],
		"versions":       versions,
		"checked_at":     checkedAt.Format(time.RFC3339),
	})
}
