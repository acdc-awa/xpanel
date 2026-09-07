package api

import (
	"net/http"

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
