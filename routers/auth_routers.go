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

	// 公开：首次设置密码（仅未初始化可用，且只允许从本机发起，防公网/局域网抢先接管）
	g.POST("auth/setup",
		middleware.LoopbackOnly,
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
