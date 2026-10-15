package routers

import (
	"github.com/chenggegedeid/catflap/api/traffic_api"
	"github.com/chenggegedeid/catflap/core/component"
	"github.com/chenggegedeid/catflap/modules/traffic"

	"github.com/gin-gonic/gin"
)

// TrafficRouters 流量统计。
//
// 保护组：流量数据能看出家里有哪些服务、什么时候在用、量有多大，
// 这些不该让未登录的人看到。
func TrafficRouters(g *gin.RouterGroup) {
	var app = traffic_api.TrafficApi{}

	// 实时速率 + 累计值。会被前端每 2 秒调一次，所以只读内存。
	g.GET("traffic/realtime", app.TrafficRealtimeView)
	// 近 N 天趋势
	g.GET("traffic/trend", app.TrafficTrendView)
}

func init() {
	component.MustContributeRoutes(traffic.Name, func(g component.RouterGroups) {
		TrafficRouters(g.Protected)
	})
}
