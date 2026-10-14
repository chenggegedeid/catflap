package routers

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// desktopSecretParam 桌面版首屏靠这个 URL 参数把免登录 secret 交给 webview。
//
// 前端读到后会立刻 replaceState 把地址栏抹掉，但服务端这边请求已经进来了：
// gin 默认的访问日志会把 RawQuery 一并输出（systemd 部署下进 journal，
// 容器里进 docker logs，本地跑进终端缓冲），等于把免登录凭证留在日志系统里。
// 所以记录日志前必须先把它剔掉。
const desktopSecretParam = "desktop_secret"

// sanitizePath 从「路径?查询串」里去掉 desktop_secret，其余参数原样保留。
// 只做字符串层面的剥离/重建，不解释路径语义。
func sanitizePath(full string) string {
	i := strings.IndexByte(full, '?')
	if i < 0 {
		return full
	}
	base, raw := full[:i], full[i+1:]
	if !strings.Contains(raw, desktopSecretParam) {
		return full
	}
	values, err := url.ParseQuery(raw)
	if err != nil {
		// 解不开就整段截掉：宁可少记几个参数，也不能把 secret 记进去
		return base
	}
	values.Del(desktopSecretParam)
	if len(values) == 0 {
		return base
	}
	return base + "?" + values.Encode()
}

// ginLogFormatter 复刻 gin 默认的访问日志格式，只多一步脱敏。
//
// 不用 gin.Default() 是因为它内部的默认 formatter 没有导出，
// 没法在中间插一层清洗，所以这里照抄一份、把 path 换成脱敏后的。
// 格式与 gin 默认保持一致，免得看日志的人换了一个不认识的样式。
func ginLogFormatter(p gin.LogFormatterParams) string {
	var statusColor, methodColor, resetColor string
	if p.IsOutputColor() {
		statusColor = p.StatusCodeColor()
		methodColor = p.MethodColor()
		resetColor = p.ResetColor()
	}
	if p.Latency > time.Minute {
		p.Latency = p.Latency.Truncate(time.Second)
	}
	return fmt.Sprintf("[GIN] %v |%s %3d %s| %13v | %15s |%s %-7s %s %#v\n%s",
		p.TimeStamp.Format("2006/01/02 - 15:04:05"),
		statusColor, p.StatusCode, resetColor,
		p.Latency,
		p.ClientIP,
		methodColor, p.Method, resetColor,
		sanitizePath(p.Path),
		p.ErrorMessage,
	)
}
