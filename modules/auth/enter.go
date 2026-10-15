package auth

import (
	"crypto/subtle"
	"strings"
	"sync"
)

// Config 鉴权配置，持久化到 config/authConfig.json
type Config struct {
	Initialized   bool   `json:"initialized"`   // 是否已设置过密码
	PasswordHash  string `json:"passwordHash"`  // pwd.HashPassword 产物
	JwtSecret     string `json:"jwtSecret"`     // 首次启动随机生成，用于签发/校验 token
	TokenTTLHours int    `json:"tokenTtlHours"` // token 有效期（小时）

	// Username 登录名。**一律存小写**，比对时也转小写 ——
	// 用户名区分大小写只会让用户自己登不上（他自己都记不清当初怎么敲的）。
	//
	// 为空表示"老配置，还没设过用户名"：登录时按 DefaultUsername 处理，
	// 并且校验用户名一律通过（保证老用户用任何用户名都能登进去，
	// 不至于因为升级把自己锁在外面）。
	Username string `json:"username,omitempty"`

	UpdatedAt string `json:"updatedAt"`
}

// DefaultUsername 没设过用户名时的登录名。
//
// 老配置迁移后就用它 —— 界面上会明确写出来，免得用户不知道该填什么。
const DefaultUsername = "admin"

// UsernameFor 这个配置实际用来登录的用户名。
// 没存过就给默认值，保证老配置升级后照样能登。
func (c Config) UsernameFor() string {
	if strings.TrimSpace(c.Username) == "" {
		return DefaultUsername
	}
	return strings.ToLower(strings.TrimSpace(c.Username))
}

// NormalizeUsername 用户名归一化：去空格 + 转小写。存和比都用它，
// 两边一致才不会出现"存的时候是 Admin、登录输 admin 却说不对"。
func NormalizeUsername(u string) string {
	return strings.ToLower(strings.TrimSpace(u))
}

// NormalizeUsernameDefault 归一化，并在为空时回落到 DefaultUsername。
//
// 给"用户没填用户名"的场合用（初始化、迁移）。注意和 UsernameFor 的区别：
// 那个是读已存的配置，这个是处理用户输入。
func NormalizeUsernameDefault(u string) string {
	n := NormalizeUsername(u)
	if n == "" {
		return DefaultUsername
	}
	return n
}

type AuthRuntime struct {
	mu     sync.RWMutex
	Config Config
}

var Runtime = &AuthRuntime{}

// desktopSecret 桌面版启动时注入，仅 webview 携带；CLI 构建保持空。
var desktopSecret string

// SetDesktopSecret 由桌面构建在启动时调用注入本地 secret。
func SetDesktopSecret(s string) {
	desktopSecret = s
}

// MatchDesktopSecret 常数时间比较请求携带的 secret；空 secret 永不匹配。
func MatchDesktopSecret(s string) bool {
	if desktopSecret == "" || s == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(desktopSecret), []byte(s)) == 1
}

// Username 当前生效的登录名（给界面显示 / 回传给前端用）。
func (r *AuthRuntime) Username() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.Config.UsernameFor()
}
