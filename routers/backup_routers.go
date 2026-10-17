package routers

import (
	"github.com/chenggegedeid/catflap/api/backup_api"
	"github.com/chenggegedeid/catflap/core/component"
	"github.com/chenggegedeid/catflap/middleware"
	"github.com/chenggegedeid/catflap/modules/backup"

	"github.com/gin-gonic/gin"
)

/**
 * 备份路由：全部走受保护组（必须登录）。
 *
 * 备份包里可能有 DDNS 的 Cloudflare token、auth 的 JWT 密钥、分组的口令哈希 ——
 * 它是**整机凭据的副本**，匿名能下载的话等于把钥匙挂门外。
 * 所以这里没有 Public 组，只有 Protected。
 */
func BackupRouters(g *gin.RouterGroup) {
	var app = backup_api.BackupApi{}

	// 先看清单再决定恢复范围（导入是覆盖性操作，先看清更稳妥）
	g.POST("backup/preview", app.PreviewView)
	// 导出下载（body 里带要导哪些模块/资产）
	g.POST("backup/export",
		middleware.BindJsonMiddleware[backup_api.ExportRequest],
		app.ExportView)
	// 导入恢复（内部会先快照，失败自动回滚）
	g.POST("backup/import", app.ImportView)
}

func init() {
	component.MustContributeRoutes(backup.Name, func(g component.RouterGroups) {
		BackupRouters(g.Protected)
	})
}
