package routers

import (
	"github.com/chenggegedeid/catflap/api/audit_api"
	"github.com/chenggegedeid/catflap/core/component"
	"github.com/chenggegedeid/catflap/middleware"
	"github.com/chenggegedeid/catflap/modules/audit"

	"github.com/gin-gonic/gin"
)

// AuditRouters 访问审计与 IP 管理。
//
// 保护组：访问记录里有来访 IP、访问了什么路径 —— 这是"谁能看到我家
// 有哪些服务、谁在扫我"的完整画像，绝不能公开。
func AuditRouters(g *gin.RouterGroup) {
	var app = audit_api.AuditApi{}

	g.GET("audit/config", app.AuditConfigView)
	g.GET("audit/ips", app.AuditIPsView)
	g.GET("audit/records", app.AuditRecordsView)

	g.POST("audit/block",
		middleware.BindJsonMiddleware[audit_api.BlockRequest], app.AuditBlockView)
	g.POST("audit/unblock",
		middleware.BindJsonMiddleware[audit_api.UnblockRequest], app.AuditUnblockView)
	g.POST("audit/block/batch",
		middleware.BindJsonMiddleware[audit_api.BatchBlockRequest], app.AuditBatchBlockView)

	g.POST("audit/settings",
		middleware.BindJsonMiddleware[audit_api.SettingsRequest], app.AuditSettingsView)
}

func init() {
	component.MustContributeRoutes(audit.Name, func(g component.RouterGroups) {
		AuditRouters(g.Protected)
	})
}
