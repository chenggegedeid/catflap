package routers

import (
	"github.com/chenggegedeid/catflap/api/notify_api"
	"github.com/chenggegedeid/catflap/core/component"
	"github.com/chenggegedeid/catflap/middleware"
	"github.com/chenggegedeid/catflap/modules/notify"

	"github.com/gin-gonic/gin"
)

// NotifyRouters 通知中心。
//
// 全部在保护组里：渠道配置里带着 token / 群机器人地址，
// 这些绝不能让未登录的人读到（哪怕掩码过，渠道名本身也是信息）。
func NotifyRouters(g *gin.RouterGroup) {
	var app = notify_api.NotifyApi{}

	g.GET("notify/config", app.NotifyConfigView)

	g.POST("notify/channel",
		middleware.BindJsonMiddleware[notify_api.ChannelSaveRequest], app.NotifyChannelSaveView)
	g.POST("notify/channel/delete",
		middleware.BindJsonMiddleware[notify_api.ChannelDeleteRequest], app.NotifyChannelDeleteView)
	g.POST("notify/channel/test",
		middleware.BindJsonMiddleware[notify_api.ChannelTestRequest], app.NotifyChannelTestView)
	// 服务里「发一条测试通知」：用服务选的渠道 + 模板试发，逐渠道回结果
	g.POST("notify/service-test",
		middleware.BindJsonMiddleware[notify_api.ServiceTestRequest], app.NotifyServiceTestView)

	g.POST("notify/rule",
		middleware.BindJsonMiddleware[notify_api.RuleSaveRequest], app.NotifyRuleSaveView)
	g.GET("notify/service-refs", app.NotifyServiceRefsView)
	g.POST("notify/rule/delete",
		middleware.BindJsonMiddleware[notify_api.RuleDeleteRequest], app.NotifyRuleDeleteView)
}

func init() {
	component.MustContributeRoutes(notify.Name, func(g component.RouterGroups) {
		NotifyRouters(g.Protected)
	})
}
