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
// 来源限制由路由上的 middleware.LoopbackOnly 承担（只允许本机发起），
// 这里不再重复判断，避免同一逻辑两处实现、改一处忘另一处。
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
