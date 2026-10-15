package auth_api

import (
	"errors"
	"strings"

	"github.com/chenggegedeid/catflap/middleware"
	"github.com/chenggegedeid/catflap/modules/auth"
	"github.com/chenggegedeid/catflap/utils/res"

	"github.com/gin-gonic/gin"
)

type ChangePasswordRequest struct {
	OldPassword string `json:"oldPassword"`
	NewPassword string `json:"newPassword"`
	// NewUsername 新的登录名。**留空表示不改用户名**，只改密码。
	// 分开放是有意的：多数时候用户只是想换密码，不该被迫重填用户名。
	NewUsername string `json:"newUsername"`
}

// ChangePasswordView 受保护：校验旧密码后更新。
// 改完所有旧 token 都失效，所以这里回一个新 token 让当前页面接着用。
func (AuthApi) ChangePasswordView(c *gin.Context) {
	cr := middleware.GetBindRequest[ChangePasswordRequest](c)
	token, err := auth.Runtime.ChangePassword(cr.OldPassword, cr.NewPassword, cr.NewUsername)
	if err != nil {
		if errors.Is(err, auth.ErrPasswordBusy) {
			res.FailTooManyRequests(c)
			return
		}
		res.FailWithMsg(err.Error(), c)
		return
	}
	msg := "密码已更新"
	if strings.TrimSpace(cr.NewUsername) != "" {
		msg = "用户名和密码已更新"
	}
	res.Ok(gin.H{"token": token, "username": auth.Runtime.Username()}, msg, c)
}
