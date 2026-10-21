package api

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/acdc-awa/xpanel/internal/pkg/util"
)

// 本文件：面板侧可选的 Xray 内核版本目录（供「自由升降级」下拉框使用）。
//
// 与 agent 的版本查询同因同源：XTLS 的发布全部标记为 prerelease，
// /releases/latest 会退回 v26.3.27，因此这里同样读 releases.atom。
// atom 只给最近若干条——够覆盖常规升级/回退；更老的版本允许界面手填。
const (
	defaultXrayRepo = "XTLS/Xray-core"
	xrayVersionTTL  = 15 * time.Minute
	xrayVersionMax  = 30
)

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

// FetchXrayVersions 从 releases.atom 取最近的内核版本（含 prerelease，按新→旧）。
func FetchXrayVersions(ctx context.Context, mirror string) ([]string, error) {
	return FetchRepoVersions(ctx, mirror, defaultXrayRepo)
}

// FetchRepoVersions 从任意仓库的 releases.atom 取最近版本（新→旧）。
// atom 包含 prerelease、不受 REST API 匿名限流，镜像候选也能代理。
func FetchRepoVersions(ctx context.Context, mirror, repo string) ([]string, error) {
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
