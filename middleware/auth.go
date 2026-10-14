package middleware

import (
	"net"
	"strings"

	"github.com/chenggegedeid/catflap/modules/auth"
	"github.com/chenggegedeid/catflap/utils/res"

	"github.com/gin-gonic/gin"
)

// DesktopSecretHeader 桌面 webview 注入的免登录 secret 头
const DesktopSecretHeader = "X-Catflap-Desktop"

// LoopbackOnly 限制接口只能从本机发起，用于「未授权即可执行」的敏感操作
// （首次初始化设置密码）。放行依据是 TCP 对端地址 Request.RemoteAddr，
// 而不是 c.ClientIP()——后者会读 X-Forwarded-For / X-Real-IP，
// 在 gin 默认信任全部代理的前提下，攻击者伪造一个 "X-Forwarded-For: 127.0.0.1"
// 就能骗过校验。这里从源头拿连接地址，伪造代理头无效。
func LoopbackOnly(c *gin.Context) {
	if !IsLoopbackRequest(c) {
		res.FailForbidden(c)
		c.Abort()
		return
	}
	c.Next()
}

// IsLoopbackRequest 本次请求是否来自回环地址（127.0.0.0/8 或 ::1）。
func IsLoopbackRequest(c *gin.Context) bool {
	return isLoopbackAddr(c.Request.RemoteAddr)
}

// isLoopbackAddr 解析 "host:port" 形式并判断 host 是否为回环。
// 解析失败时退化为整体当作 host 判断，避免因格式异常而误放行/误拒。
func isLoopbackAddr(remoteAddr string) bool {
	host := remoteAddr
	if h, _, err := net.SplitHostPort(remoteAddr); err == nil {
		host = h
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// AuthMiddleware 保护受限接口：桌面 secret 或有效 token 之一通过即放行。
// 未初始化返回 428 引导前端去设置密码；否则 401。
func AuthMiddleware(c *gin.Context) {
	if !auth.Runtime.IsInitialized() {
		res.FailNeedInit(c)
		c.Abort()
		return
	}

	if IsAuthed(c) {
		c.Next()
		return
	}

	res.FailUnauthorized(c)
	c.Abort()
}

// IsAuthed 这次请求有没有登录。和 AuthMiddleware 放行用的是同一套判断，
// 公开接口（比如 auth/status）想知道当前是不是登录态时复用它，
// 免得两边各写一份、改一处忘另一处。
func IsAuthed(c *gin.Context) bool {
	// 桌面免登录通道：仅 wails webview 携带 secret，CLI 构建 secret 为空永不匹配
	if secret := c.GetHeader(DesktopSecretHeader); secret != "" && auth.MatchDesktopSecret(secret) {
		return true
	}

	// token 校验：Authorization: Bearer <token>
	token := bearerToken(c)
	return token != "" && auth.Runtime.ValidateToken(token)
}

func bearerToken(c *gin.Context) string {
	h := c.GetHeader("Authorization")
	const prefix = "Bearer "
	if strings.HasPrefix(h, prefix) {
		return strings.TrimSpace(h[len(prefix):])
	}
	return ""
}
