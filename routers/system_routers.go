package routers

import (
	"github.com/chenggegedeid/catflap/api/system_api"
	"github.com/chenggegedeid/catflap/core/component"
	"github.com/chenggegedeid/catflap/middleware"
	"github.com/chenggegedeid/catflap/modules/system"
	"github.com/chenggegedeid/catflap/modules/update"

	"github.com/gin-gonic/gin"
)

// SystemRouters 系统信息：版本号。公开 —— 界面一加载就要显示它。
func SystemRouters(g *gin.RouterGroup) {
	var app = system_api.SystemApi{}

	// 公开：获取版本号
	g.GET("version", app.GetVersion)
}

// SystemRoutersProtected 运行日志。日志里有内网地址、域名、后端错误，不能公开。
func SystemRoutersProtected(g *gin.RouterGroup) {
	var app = system_api.SystemApi{}

	g.GET("system/log/days", app.LogDaysView)
	g.GET("system/log", app.LogListView)
	// 分类摘要：只回各模块的条数与异常数，折叠态用（不读内容，省得打开页面就卡）
	g.GET("system/log/summary", app.LogSummaryView)

	// 功能管理：有哪些组件、能不能开关、现在什么状态
	g.GET("system/components", app.ListComponents)
	g.POST(
		"system/component/enable",
		middleware.BindJsonMiddleware[system_api.ComponentEnableRequest],
		app.SetComponentEnabled,
	)
}

// UpdateRouters 一键更新：换二进制、重启进程，必须登录。
//
// 从 SystemRoutersProtected 里拆出来单独一个函数，是因为这些接口属于
// **update 组件**而不是 system 组件。归属哪个组件决定了「在功能页上关掉它」
// 能不能真的让接口失效 —— 混在 system 里的话，用户关掉「一键更新」，
// 接口照样能用，开关就成了摆设。
func UpdateRouters(g *gin.RouterGroup) {
	var app = system_api.SystemApi{}

	g.GET("system/update", app.UpdateStatusView)
	g.POST("system/update/check", app.UpdateCheckView)
	g.POST("system/update/install", app.UpdateInstallView)
	g.POST("system/update/cancel", app.UpdateCancelView)
	g.POST("system/update/upload", app.UpdateUploadView)
	g.PUT(
		"system/update/config",
		middleware.BindJsonMiddleware[system_api.UpdateConfigRequest],
		app.UpdateConfigView,
	)
}

func init() {
	component.MustContributeRoutes(system.Name, func(g component.RouterGroups) {
		SystemRouters(g.Public)
		SystemRoutersProtected(g.Protected)
	})
	component.MustContributeRoutes(update.Name, func(g component.RouterGroups) {
		UpdateRouters(g.Protected)
	})
}
