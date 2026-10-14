package auth_api

import (
	"errors"
	"github.com/chenggegedeid/catflap/middleware"
	"github.com/chenggegedeid/catflap/modules/auth"
	"github.com/chenggegedeid/catflap/utils/res"

	"github.com/gin-gonic/gin"
)

type LoginRequest struct {
	Password string `json:"password"`
	// Remember 勾了「记住我」→ 签发 7 天有效的 token；不勾按默认 24 小时
	Remember bool `json:"remember"`
}

// LoginView 校验密码，成功返回 token
func (AuthApi) LoginView(c *gin.Context) {
	cr := middleware.GetBindRequest[LoginRequest](c)
	token, err := auth.Runtime.Login(cr.Password, cr.Remember)
	if err != nil {
		if errors.Is(err, auth.ErrLoginLimited) {
			res.FailRateLimited(c)
			return
		}
		if errors.Is(err, auth.ErrPasswordBusy) {
			res.FailTooManyRequests(c)
			return
		}
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithData(gin.H{"token": token}, c)
}
