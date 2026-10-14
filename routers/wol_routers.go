package routers

import (
	"github.com/chenggegedeid/catflap/api/wol_api"
	"github.com/chenggegedeid/catflap/core/component"
	"github.com/chenggegedeid/catflap/middleware"
	"github.com/chenggegedeid/catflap/modules/wol"

	"github.com/gin-gonic/gin"
)

func WolRouters(g *gin.RouterGroup) {
	var app = wol_api.WolApi{}

	// 机器：查 / 增 / 改 / 删
	g.GET(
		"wol/targets",
		app.WolListView,
	)
	g.POST(
		"wol/target/add",
		middleware.BindJsonMiddleware[wol_api.WolSaveRequest],
		app.WolAddView,
	)
	g.PUT(
		"wol/target/update",
		middleware.BindJsonMiddleware[wol_api.WolSaveRequest],
		app.WolUpdateView,
	)
	g.DELETE(
		"wol/target/delete",
		middleware.BindJsonMiddleware[wol_api.WolDeleteRequest],
		app.WolDeleteView,
	)
	// 唤醒：不写配置，只发一个包，所以用 POST 而不是 PUT
	g.POST(
		"wol/wake",
		middleware.BindJsonMiddleware[wol_api.WolWakeRequest],
		app.WolWakeView,
	)
}

// 把本模块的挂载交给组件注册表，routers/enter.go 不再需要列它。
func init() {
	component.MustContributeRoutes(wol.Name, func(g component.RouterGroups) {
		WolRouters(g.Protected)
	})
}
