package api

import (
	"github.com/gin-gonic/gin"

	"github.com/acdc-awa/xpanel/internal/master/protocols"
	"github.com/acdc-awa/xpanel/internal/pkg/util"
)

// AdminProtocols GET /api/v1/admin/protocols
// 返回协议能力描述符 + 字段声明 + 交叉规则 + 方言映射（前端据此渲染表单/提示）。
func (d *Deps) AdminProtocols(c *gin.Context) {
	sets := protocols.AllFieldSets()
	items := make([]gin.H, 0, len(sets))
	for _, fs := range sets {
		item := gin.H{"field_set": fs}
		if p := protocols.Find(fs.Protocol); p != nil {
			item["descriptor"] = p.Descriptor()
		}
		items = append(items, item)
	}
	util.OK(c, gin.H{"protocols": items})
}
