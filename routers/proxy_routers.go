package routers

import (
	"github.com/chenggegedeid/catflap/api/proxy_api"
	"github.com/chenggegedeid/catflap/core/component"
	"github.com/chenggegedeid/catflap/middleware"
	"github.com/chenggegedeid/catflap/modules/proxy"

	"github.com/gin-gonic/gin"
)

func ProxyRouters(g *gin.RouterGroup) {
	var app = proxy_api.ProxyApi{}

	// 入口设置 + 监听状态 + 站点表，一次给全
	g.GET(
		"proxy/config",
		app.ProxyConfigView,
	)

	// 默认入口：HTTP 端口、HTTPS 端口、默认证书、主域名
	g.POST(
		"proxy/entry",
		middleware.BindJsonMiddleware[proxy_api.EntrySaveRequest],
		app.ProxyEntrySaveView,
	)

	// 访问日志单独一个开关，不碰监听
	g.POST(
		"proxy/accesslog",
		middleware.BindJsonMiddleware[proxy_api.AccessLogRequest],
		app.ProxyAccessLogView,
	)

	// 站点增 / 改 / 删
	g.POST(
		"proxy/site",
		middleware.BindJsonMiddleware[proxy_api.SiteSaveRequest],
		app.ProxySiteAddView,
	)
	g.PUT(
		"proxy/site",
		middleware.BindJsonMiddleware[proxy_api.SiteSaveRequest],
		app.ProxySiteUpdateView,
	)
	g.DELETE(
		"proxy/site",
		middleware.BindJsonMiddleware[proxy_api.SiteDeleteRequest],
		app.ProxySiteDeleteView,
	)

	// 立即拨一次后端，不要求先保存
	g.POST(
		"proxy/site/test",
		middleware.BindJsonMiddleware[proxy_api.SiteSaveRequest],
		app.ProxySiteTestView,
	)

	// 站点表单的可选项（域名、网段、本机地址）。在 api 层聚合，
	// 不让 proxy 依赖 ddns —— 见 proxy_site_options.go 的说明。
	g.GET("proxy/site-options", app.ProxySiteOptionsView)

	// 站点的一键开关（启用 / 显示在主页），和穿透服务卡片的操作对齐
	g.POST("proxy/site/flag",
		middleware.BindJsonMiddleware[proxy_api.SiteFlagRequest], app.ProxySiteFlagView)

	// 局域网扫描：和穿透那边共用 core/lanscan 的实现
	g.GET("proxy/lan/subnets", app.ProxyLanSubnetsView)
	g.POST("proxy/lan/scan",
		middleware.BindJsonMiddleware[proxy_api.ProxyLanScanRequest], app.ProxyLanScanView)
}

// 把本模块的挂载交给组件注册表，routers/enter.go 不再需要列它。
func init() {
	component.MustContributeRoutes(proxy.Name, func(g component.RouterGroups) {
		ProxyRouters(g.Protected)
	})
}
