package res

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type Response struct {
	Code int    `json:"code"`
	Data any    `json:"data"`
	Msg  string `json:"msg"`
}

func Ok(data any, msg string, c *gin.Context) {
	c.JSON(200, Response{
		Code: 0,
		Data: data,
		Msg:  msg,
	})
}

func OkWithMsg(msg string, c *gin.Context) {
	Ok(gin.H{}, msg, c)
}

func OkWithData(data any, c *gin.Context) {
	Ok(data, "成功", c)
}

func OkWithList(list any, count int64, c *gin.Context) {
	Ok(map[string]any{
		"list":  list,
		"count": count,
	}, "成功", c)
}
func Fail(code int, msg string, c *gin.Context) {
	c.JSON(200, Response{
		Code: code,
		Data: gin.H{},
		Msg:  msg,
	})
}

func FailWithMsg(msg string, c *gin.Context) {
	Fail(7, msg, c)
}

func FailWithError(err error, c *gin.Context) {
	msg := err.Error()
	Fail(7, msg, c)
}

// 鉴权约定码：401 未登录/token 失效，403 仅限本机，428 需先初始化（设置密码）
const (
	CodeUnauthorized    = 401
	CodeForbidden       = 403
	CodeNeedInit        = 428
	CodeTooManyRequests = 429
)

func FailUnauthorized(c *gin.Context) {
	Fail(CodeUnauthorized, "未登录或登录已失效", c)
}

func FailNeedInit(c *gin.Context) {
	Fail(CodeNeedInit, "系统尚未初始化", c)
}

// FailForbidden 回真正的 HTTP 403，让调用方（curl / 前端）一眼能看出是被拒而非业务失败
func FailForbidden(c *gin.Context) {
	c.JSON(http.StatusForbidden, Response{
		Code: CodeForbidden,
		Data: gin.H{},
		Msg:  "该操作仅允许在运行本程序的本机上发起，请回到本机操作",
	})
}

func FailTooManyRequests(c *gin.Context) {
	c.JSON(http.StatusTooManyRequests, Response{
		Code: CodeTooManyRequests,
		Data: gin.H{},
		Msg:  "密码校验繁忙，请稍后重试",
	})
}

// FailRateLimited 登录被限速/封禁。
//
// ⚠️ 以前这里把「3 分钟 13 次」写死在字符串里，配置一改文案就成了假话。
// 现在由调用方把真实的剩余时间传进来 —— 用户看到的是真话，也知道要等多久
// （不知道还要等多久的等待最折磨人）。
func FailRateLimited(c *gin.Context) {
	FailRateLimitedWith("", c)
}

func FailRateLimitedWith(msg string, c *gin.Context) {
	if strings.TrimSpace(msg) == "" {
		msg = "登录失败次数过多，请稍后再试"
	}
	c.JSON(http.StatusTooManyRequests, Response{
		Code: CodeTooManyRequests,
		Data: gin.H{},
		Msg:  msg,
	})
}
