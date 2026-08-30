package api

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
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

var (
	updateMu     sync.Mutex                   // apply 幂等锁（并发/重复触发拒绝）
	updateCfgCfg = "/app/configs/config.yaml" // 容器形态固定配置路径
)

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

// AdminUpdateApply POST /api/v1/admin/update/apply
// body: {"version": "vX.Y.Z"}（空 = 最新 release）
func (d *Deps) AdminUpdateApply(c *gin.Context) {
	if d.Cfg == nil || !d.Cfg.Update.Enabled {
		util.BadRequest(c, "面板内更新已禁用（config update.enabled=false）")
		return
	}
	if !updateMu.TryLock() {
		util.Fail(c, 409, "更新正在进行中，请稍后再试")
		return
	}
	defer updateMu.Unlock()

	var req struct {
		Version string `json:"version"`
	}
	_ = c.ShouldBindJSON(&req)

	// 容器形态硬约束：仅当程序运行在挂载形态（/app/master 可写位置存在）时允许自更新
	appRoot := "/app"
	masterPath := filepath.Join(appRoot, "master")
	if _, err := os.Stat(masterPath); err != nil {
		util.BadRequest(c, "未检测到容器挂载形态（/app/master 不存在），面板内更新仅在 compose 部署下可用")
		return
	}

	// 1. 解析更新源：指定版本走直链模板，空走 releases/latest
	var (
		rel        *latestRelease
		tarballURL string
		shaURL     string
		err        error
	)
	if req.Version != "" {
		tarballURL, shaURL = d.assetURLs(req.Version)
	} else {
		rel, tarballURL, shaURL, err = d.resolveLatest(c.Request.Context())
		if err != nil {
			util.Fail(c, 502, "查询更新源失败: "+err.Error())
			return
		}
	}
	if tarballURL == "" {
		util.BadRequest(c, "更新源未提供当前架构（"+runtime.GOARCH+"）的安装包")
		return
	}

	// 2. 下载 + sha256 校验
	stage, err := os.MkdirTemp(d.dataDir(), "update-staging-")
	if err != nil {
		util.ServerError(c, "创建暂存目录失败: "+err.Error())
		return
	}
	defer os.RemoveAll(stage)

	tarball := filepath.Join(stage, "package.tar.gz")
	log.Printf("update: 下载 %s", tarballURL)
	if err := downloadFile(c.Request.Context(), tarballURL, tarball); err != nil {
		util.Fail(c, 502, "下载更新包失败: "+err.Error())
		return
	}
	hash, err := sha256File(tarball)
	if err != nil {
		util.ServerError(c, "计算校验失败: "+err.Error())
		return
	}
	if err := d.verifyChecksum(c.Request.Context(), shaURL, tarball, hash); err != nil {
		util.BadRequest(c, "更新包校验失败（拒绝安装）: "+err.Error())
		return
	}

	// 3. 解压（白名单提取 master + web/dist，防路径穿越）
	unpacked := filepath.Join(stage, "unpacked")
	if err := extractTarGz(tarball, unpacked); err != nil {
		util.BadRequest(c, "解压更新包失败: "+err.Error())
		return
	}
	newBin := filepath.Join(unpacked, "master", "master")
	newDist := filepath.Join(unpacked, "web", "dist")
	if _, err := os.Stat(newBin); err != nil {
		util.BadRequest(c, "更新包缺少 master/master（包结构不符）")
		return
	}
	if _, err := os.Stat(newDist); err != nil {
		util.BadRequest(c, "更新包缺少 web/dist（包结构不符）")
		return
	}

	// 4. 冒烟：新二进制 -self-test（不连库，无副作用；挡架构错误/损坏/配置不兼容）
	out, err := exec.Command(newBin, "-self-test", "-config", updateCfgCfg).CombinedOutput()
	if err != nil {
		util.BadRequest(c, fmt.Sprintf("新版本二进制自检未通过，拒绝替换: %v / %s", err, strings.TrimSpace(string(out))))
		return
	}
	log.Printf("update: 自检通过: %s", strings.TrimSpace(string(out)))

	version := req.Version
	if version == "" && rel != nil {
		version = rel.TagName
	}

	// 5. 备份当前版本 + 落待确认标记（供 entrypoint 回滚判定）
	cur, err := os.ReadFile(masterPath)
	if err != nil {
		util.ServerError(c, "读取当前版本失败: "+err.Error())
		return
	}
	if err := os.WriteFile(masterPath+".prev", cur, 0o755); err != nil {
		util.ServerError(c, "备份当前版本失败: "+err.Error())
		return
	}
	webDist := filepath.Join(appRoot, "web", "dist")
	if err := copyDir(webDist, webDist+".prev"); err != nil {
		util.ServerError(c, "备份前端失败: "+err.Error())
		return
	}
	if err := os.WriteFile(filepath.Join(appRoot, ".update-pending"), []byte(version), 0o644); err != nil {
		util.ServerError(c, "写入更新标记失败: "+err.Error())
		return
	}

	// 6. 原子替换（staging 与 /app 同文件系统，rename 原子）
	if err := os.Rename(newBin, masterPath); err != nil {
		util.ServerError(c, "替换 master 失败: "+err.Error())
		return
	}
	if err := os.RemoveAll(webDist); err != nil {
		util.ServerError(c, "清理旧前端失败: "+err.Error())
		return
	}
	if err := os.Rename(newDist, webDist); err != nil {
		util.ServerError(c, "替换前端失败: "+err.Error())
		return
	}

	// 7. 触发优雅退出：SIGTERM → 既有 signal handler 关停全链路 → 进程退出 0 →
	//    Docker 检测到容器停止（非人工 stop）→ unless-stopped 拉起新版本
	log.Printf("update: 已替换为 %s（%s），1s 后触发重启", version, runtime.GOARCH)
	go func() {
		time.Sleep(time.Second)
		if p, err := os.FindProcess(os.Getpid()); err == nil {
			if err := p.Signal(syscall.SIGTERM); err != nil {
				log.Printf("update: 自杀信号发送失败: %v", err)
			}
		}
	}()
	util.OK(c, gin.H{"ok": true, "version": version, "message": "更新已就绪，容器即将重启以应用新版本"})
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
	prefix := "https://github.com"
	if d.Cfg != nil && d.Cfg.Update.Mirror != "" {
		prefix = strings.TrimSuffix(d.Cfg.Update.Mirror, "/")
	}
	tarballURL, shaURL := "", ""
	for _, a := range rel.Assets {
		if a.Name == want {
			tarballURL = strings.Replace(a.BrowserDownloadURL, "https://github.com", prefix, 1)
			shaURL = strings.TrimSuffix(tarballURL, ".tar.gz") + ".tar.gz.sha256"
			break
		}
	}
	return &rel, tarballURL, shaURL, nil
}

// assetURLs 按指定版本推导直链（--version 场景，与 install.sh URL 模板一致）。
func (d *Deps) assetURLs(version string) (string, string) {
	base := "https://github.com"
	if d.Cfg != nil && d.Cfg.Update.Mirror != "" {
		base = strings.TrimSuffix(d.Cfg.Update.Mirror, "/")
	}
	u := fmt.Sprintf("%s/%s/releases/download/%s/xpanel-master-%s-linux-%s.tar.gz",
		base, d.repo(), version, version, runtime.GOARCH)
	return u, u + ".sha256"
}

func (d *Deps) repo() string {
	if d.Cfg != nil && d.Cfg.Update.Repo != "" {
		return d.Cfg.Update.Repo
	}
	return "acdc-awa/xpanel"
}

// dataDir 容器形态的数据目录（staging 需与 /app 同文件系统才能原子 rename）。
func (d *Deps) dataDir() string {
	if d.Cfg != nil && d.Cfg.DB.Driver == "sqlite" && d.Cfg.DB.DSN != "" {
		if dir := filepath.Dir(d.Cfg.DB.DSN); dir != "" && dir != "." {
			return dir
		}
	}
	return "/app/data"
}

// verifyChecksum 用 release 的 .sha256 强制校验（与 install.sh 相同信任模型）。
func (d *Deps) verifyChecksum(ctx context.Context, shaURL, tarball, actual string) error {
	if shaURL == "" {
		return fmt.Errorf("缺少校验和地址")
	}
	var buf strings.Builder
	if err := downloadTo(ctx, shaURL, &buf); err != nil {
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

// downloadFile 下载 URL 到本地文件。
func downloadFile(ctx context.Context, url, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return downloadTo(ctx, url, f)
}

// downloadTo 下载 URL 内容写入 writer。
func downloadTo(ctx context.Context, url string, w io.Writer) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载 %s 返回 %d", url, resp.StatusCode)
	}
	_, err = io.Copy(w, resp.Body)
	return err
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
			return fmt.Errorf("非法路径 %q", hdr.Name)
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
