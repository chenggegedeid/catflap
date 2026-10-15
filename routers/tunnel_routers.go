package routers

import (
	"github.com/chenggegedeid/catflap/api/tunnel_api"
	"github.com/chenggegedeid/catflap/core/component"
	"github.com/chenggegedeid/catflap/middleware"
	"github.com/chenggegedeid/catflap/modules/tunnel"

	"github.com/gin-gonic/gin"
)

// TunnelRouters Cloudflare Tunnel。
//
// 保护组：隧道 token 是这条通道的全部凭证，拿到就能以你的名义起一条隧道。
func TunnelRouters(g *gin.RouterGroup) {
	var app = tunnel_api.TunnelApi{}

	g.GET("tunnel/config", app.TunnelConfigView)
	g.POST("tunnel",
		middleware.BindJsonMiddleware[tunnel_api.SaveRequest], app.TunnelSaveView)
	g.POST("tunnel/restart", app.TunnelRestartView)
}

func init() {
	component.MustContributeRoutes(tunnel.Name, func(g component.RouterGroups) {
		TunnelRouters(g.Protected)
	})
}
