package routers

import (
	"github.com/chenggegedeid/catflap/api/auth_api"
	"github.com/chenggegedeid/catflap/core/component"
	"github.com/chenggegedeid/catflap/middleware"
	"github.com/chenggegedeid/catflap/modules/auth"

	"github.com/gin-gonic/gin"
)

func AuthRouters(g *gin.RouterGroup) {
	var app = auth_api.AuthApi{}

	// 公开：是否已初始化
	g.GET("auth/status", app.StatusView)

	// 公开：首次设置密码。
	//
	// 这里**刻意不限制来源**（早期版本用 middleware.LoopbackOnly 只允许本机）。
	// 取舍写在 docs/decisions/0005-初始化接口放开来源限制.md：
	// 在「装好还没初始化」这个窗口里，同网段任何人都能打开页面抢先设置管理员
	// 密码，而改密码要旧密码 —— 也就是说机器会被接管且拿不回来。
	//
	// 之所以接受：这个窗口是自建服务的常规形态（同样的取舍在 NAS / 路由器一类的
	// 产品里很常见），而限制来源带来的代价是「必须跑到机器跟前开浏览器」，
	// 对无头设备 / 远程装机的场景太别扭。想要更严的部署可以自己加一层前置认证，
	// 或者装完立刻初始化。
	g.POST("auth/setup",
		middleware.BindJsonMiddleware[auth_api.SetupRequest], app.SetupView)

	// 公开：登录
	g.POST("auth/login",
		middleware.BindJsonMiddleware[auth_api.LoginRequest], app.LoginView)

	// 受保护：修改密码
	protected := g.Group("", middleware.AuthMiddleware)
	protected.PUT("auth/password",
		middleware.BindJsonMiddleware[auth_api.ChangePasswordRequest], app.ChangePasswordView)
}

// 把本模块的挂载交给组件注册表，routers/enter.go 不再需要列它。
func init() {
	component.MustContributeRoutes(auth.Name, func(g component.RouterGroups) {
		AuthRouters(g.Public)
	})
}
