package auth_api

import (
	"errors"
	"github.com/chenggegedeid/catflap/middleware"
	"github.com/chenggegedeid/catflap/modules/auth"
	"github.com/chenggegedeid/catflap/utils/res"

	"github.com/gin-gonic/gin"
)

type SetupRequest struct {
	Password string `json:"password"`
}

// SetupView 公开但仅在未初始化时可用：设置 admin 密码，成功后返回 token 直接进后台
//
// **不限制来源**：本机、局域网、公网来的请求一视同仁。早先版本用
// middleware.LoopbackOnly 只放行回环地址，后来去掉了 —— 取舍见
// docs/decisions/0005-初始化接口放开来源限制.md。
//
// 未初始化期间谁先到谁就能设密码，这是有意的：这个窗口是自建服务的常规形态，
// 而「必须跑到机器跟前开浏览器」对无头设备 / 远程装机太别扭。
// 想更严就装完立刻初始化，或者在前面挂一层认证。
func (AuthApi) SetupView(c *gin.Context) {
	cr := middleware.GetBindRequest[SetupRequest](c)
	if err := auth.Runtime.Setup(cr.Password); err != nil {
		if errors.Is(err, auth.ErrPasswordBusy) {
			res.FailTooManyRequests(c)
			return
		}
		res.FailWithMsg(err.Error(), c)
		return
	}
	token, err := auth.Runtime.Login(cr.Password, false)
	if err != nil {
		if errors.Is(err, auth.ErrPasswordBusy) {
			res.FailTooManyRequests(c)
			return
		}
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithData(gin.H{"token": token}, c)
}
