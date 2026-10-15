package auth_api

import (
	"errors"
	"github.com/chenggegedeid/catflap/middleware"
	"github.com/chenggegedeid/catflap/modules/auth"
	"github.com/chenggegedeid/catflap/utils/res"

	"github.com/gin-gonic/gin"
)

type SetupRequest struct {
	// Username 登录名。留空用 auth.DefaultUsername（"admin"）——
	// 老的安装流程只有密码，不能因为没传就失败
	Username string `json:"username"`
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
	if err := auth.Runtime.Setup(cr.Username, cr.Password); err != nil {
		if errors.Is(err, auth.ErrPasswordBusy) {
			res.FailTooManyRequests(c)
			return
		}
		res.FailWithMsg(err.Error(), c)
		return
	}
	// 初始化后立刻登录。这里用**空 remoteAddr**：刚刚设的密码，
	// 不可能撞上限速；而且这是本机第一次登录，不该受任何桶影响。
	token, err := auth.Runtime.Login(cr.Username, cr.Password, "", false)
	if err != nil {
		if errors.Is(err, auth.ErrPasswordBusy) {
			res.FailTooManyRequests(c)
			return
		}
		res.FailWithMsg(err.Error(), c)
		return
	}
	// 把实际生效的用户名回给前端：留空时前端不知道默认成了什么，
	// 下次登录会懵
	res.OkWithData(gin.H{"token": token, "username": auth.NormalizeUsernameDefault(cr.Username)}, c)
}
