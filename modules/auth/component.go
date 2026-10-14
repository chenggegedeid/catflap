package auth

import (
	"context"

	"github.com/chenggegedeid/catflap/core/component"
)

// Name 组件名。与 routers/auth_routers.go 里 ContributeRoutes 用的名字一致。
const Name = "auth"

// authComponent 把认证模块接到组件注册表上。
//
// 刻意**不实现 Enablable**：关掉它等于把登录入口一起关掉，用户再也进不了后台，
// 只能上机器改配置文件救回来。「能不能关」和「关了会怎样」要一起看 ——
// 这种开关不该摆在界面上。
type authComponent struct{}

func (authComponent) Name() string { return Name }

// Init 无 ctx 可用：密码哈希与 token 密钥都在本机文件里，没有网络等待。
// 这里显式忽略而不是传下去，是为了让「auth 不做任何可阻塞的事」这条约定
// 在签名的层面就看得出来。
func (authComponent) Init(context.Context) error { return InitAuth() }

func init() { component.MustRegister(authComponent{}) }
