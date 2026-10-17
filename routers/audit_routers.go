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
	// 概览：指标卡 + 趋势 + 攻击类型 + 地区 + 站点，一个接口一次给齐
	g.GET("audit/overview", app.AuditOverviewView)
	g.GET("audit/ips", app.AuditIPsView)
	g.GET("audit/records", app.AuditRecordsView)
	// 登录尝试（谁在试着进后台）
	g.GET("audit/logins", app.AuditLoginsView)

	g.POST("audit/block",
		middleware.BindJsonMiddleware[audit_api.BlockRequest], app.AuditBlockView)
	g.POST("audit/unblock",
		middleware.BindJsonMiddleware[audit_api.UnblockRequest], app.AuditUnblockView)
	g.POST("audit/block/batch",
		middleware.BindJsonMiddleware[audit_api.BatchBlockRequest], app.AuditBatchBlockView)

	g.POST("audit/settings",
		middleware.BindJsonMiddleware[audit_api.SettingsRequest], app.AuditSettingsView)
	// 保存「永不封禁」名单（白名单）。单独一个接口是给审计页的
	// 「IP 名单」区块用的：它只改名单与自动拉黑，不动保留天数那些。
	g.POST("audit/whitelist",
		middleware.BindJsonMiddleware[audit_api.WhitelistRequest], app.AuditWhitelistView)
}

func init() {
	component.MustContributeRoutes(audit.Name, func(g component.RouterGroups) {
		AuditRouters(g.Protected)
	})
}
