package auth_api

import (
	"errors"
	"github.com/chenggegedeid/catflap/middleware"
	"github.com/chenggegedeid/catflap/modules/auth"
	"github.com/chenggegedeid/catflap/utils/res"

	"github.com/gin-gonic/gin"
)

type LoginRequest struct {
	// Username 登录名。**留空也能登**：老版本只有密码、没有用户名，
	// 那些配置里不存在这个概念，拿它去比对只会把升级上来的用户锁在外面。
	Username string `json:"username"`
	Password string `json:"password"`
	// Remember 勾了「记住我」→ 签发 7 天有效的 token；不勾按默认 24 小时
	Remember bool `json:"remember"`
}

// LoginView 校验用户名和密码，成功返回 token
func (AuthApi) LoginView(c *gin.Context) {
	cr := middleware.GetBindRequest[LoginRequest](c)

	// remoteAddr 用 TCP 对端地址做限速分桶：c.ClientIP() 会读 XFF，
	// 攻击者伪造头就能换一个桶绕过限速（见 auth.clientKey 的说明）
	token, err := auth.Runtime.Login(cr.Username, cr.Password, c.Request.RemoteAddr, cr.Remember)
	if err != nil {
		if errors.Is(err, auth.ErrLoginLimited) {
			res.FailRateLimited(c)
			return
		}
		if errors.Is(err, auth.ErrPasswordBusy) {
			res.FailTooManyRequests(c)
			return
		}
		// 用户名或密码错都返回同一句，不告诉对方"用户名对了、密码错了" ——
		// 那等于帮忙确认了用户名存在，爆破就成功了一半
		if errors.Is(err, auth.ErrWrongUsername) || errors.Is(err, auth.ErrWrongPassword) {
			res.FailWithMsg(auth.ErrWrongPassword.Error(), c)
			return
		}
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithData(gin.H{"token": token}, c)
}
