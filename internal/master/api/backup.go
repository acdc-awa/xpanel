package api

import (
	"net/http"
	"os"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/acdc-awa/xpanel/internal/pkg/util"
)

// AdminCreateBackup 手动触发一次备份。
func (d *Deps) AdminCreateBackup(c *gin.Context) {
	info, err := d.Backup.Snapshot()
	if err != nil {
		util.Fail(c, http.StatusInternalServerError, "备份失败: "+err.Error())
		return
	}
	util.OK(c, info)
}

// AdminListBackups 备份列表（倒序）。
func (d *Deps) AdminListBackups(c *gin.Context) {
	items, err := d.Backup.List()
	if err != nil {
		util.Fail(c, http.StatusInternalServerError, "读取备份列表失败: "+err.Error())
		return
	}
	util.OK(c, gin.H{"items": items})
}

// AdminDownloadBackup 下载指定备份文件。
func (d *Deps) AdminDownloadBackup(c *gin.Context) {
	name := c.Param("file")
	path, err := d.Backup.OpenFile(name)
	if err != nil {
		util.Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	c.FileAttachment(path, name)
}

// AdminDeleteBackup DELETE /api/v1/admin/backup/:file —— 删除单份备份（自动轮转之外的显式清理）。
func (d *Deps) AdminDeleteBackup(c *gin.Context) {
	if err := d.Backup.Delete(c.Param("file")); err != nil {
		util.Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	util.OK(c, gin.H{"file": c.Param("file")})
}

// AdminUploadBackup POST /api/v1/admin/backup/upload —— 上传本地 .db 备份入库（multipart 字段 file）。
// 上传即校验（完整性 / 必需表 / schema 兼容），通过后成为备份列表中的普通一份。
func (d *Deps) AdminUploadBackup(c *gin.Context) {
	fh, err := c.FormFile("file")
	if err != nil {
		util.Fail(c, http.StatusBadRequest, "缺少上传文件（字段名 file）")
		return
	}
	f, err := fh.Open()
	if err != nil {
		util.Fail(c, http.StatusBadRequest, "打开上传文件失败: "+err.Error())
		return
	}
	defer f.Close()
	info, err := d.Backup.Upload(f, fh.Size)
	if err != nil {
		util.Fail(c, http.StatusBadRequest, "上传失败: "+err.Error())
		return
	}
	util.OK(c, info)
}

// AdminRestoreBackup POST /api/v1/admin/backup/restore —— 用指定备份替换当前数据库。
// 恢复在下次进程启动前由 backup.ApplyPendingRestore 完成，故此处安排后触发优雅重启
// （容器 restart: unless-stopped / systemd 拉起后加载恢复后的库）。
func (d *Deps) AdminRestoreBackup(c *gin.Context) {
	var req struct {
		File string `json:"file" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.Fail(c, http.StatusBadRequest, "file 不能为空")
		return
	}
	safety, err := d.Backup.ScheduleRestore(req.File)
	if err != nil {
		util.Fail(c, http.StatusBadRequest, "恢复失败: "+err.Error())
		return
	}
	util.OK(c, gin.H{"file": req.File, "safety_backup": safety, "restarting": true})
	go func() {
		// 留出前端接收响应/展示提示的时间窗，再发信号触发优雅退出。
		time.Sleep(2 * time.Second)
		if p, err := os.FindProcess(os.Getpid()); err == nil {
			_ = p.Signal(syscall.SIGTERM)
		}
	}()
}
