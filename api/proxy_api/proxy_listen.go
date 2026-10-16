package proxy_api

import (
	"github.com/chenggegedeid/catflap/middleware"
	"github.com/chenggegedeid/catflap/modules/proxy"
	proxymodel "github.com/chenggegedeid/catflap/modules/proxy/model"
	"github.com/chenggegedeid/catflap/utils/res"

	"github.com/gin-gonic/gin"
)

// EntrySaveRequest 默认入口设置，对应 nginx 的 listen 80 / listen 443 ssl
type EntrySaveRequest struct {
	Enabled bool `json:"enabled"`
	// HTTPPorts / HTTPSPorts 入口端口，**可以多个**（新前端传这两个）
	HTTPPorts  []int `json:"httpPorts"`
	HTTPSPorts []int `json:"httpsPorts"`
	// HTTPPort / HTTPSPort 单个端口，老前端兼容用。
	// 新前端传了数组时以数组为准，见 ProxyEntrySaveView。
	HTTPPort  int  `json:"httpPort"`  // 0 = 不开 HTTP 入口
	HTTPSPort int  `json:"httpsPort"` // 0 = 不开 HTTPS 入口
	CertID    uint `json:"certId"`    // 0 表示按 SNI 自动匹配

	BaseDomain string `json:"baseDomain"` // 主域名，加站点时只填前缀就能补全
}

// ProxyEntrySaveView 保存默认入口设置并立刻生效
func (ProxyApi) ProxyEntrySaveView(c *gin.Context) {
	cr := middleware.GetBindRequest[EntrySaveRequest](c)

	// 端口用数组；老前端只传单个 httpPort/httpsPort 时也能工作
	httpPorts := cr.HTTPPorts
	if len(httpPorts) == 0 && cr.HTTPPort > 0 {
		httpPorts = []int{cr.HTTPPort}
	}
	httpsPorts := cr.HTTPSPorts
	if len(httpsPorts) == 0 && cr.HTTPSPort > 0 {
		httpsPorts = []int{cr.HTTPSPort}
	}

	err := proxy.SetEntry(proxy.EntrySettings{
		Enabled:    cr.Enabled,
		HTTPPorts:  httpPorts,
		HTTPSPorts: httpsPorts,
		CertID:     cr.CertID,
		BaseDomain: cr.BaseDomain,
	})
	if err != nil {
		// 配置已经存下了，没存下的只是「跑起来」这件事。
		// 照实报错，前端刷新后能看到同一句话挂在对应端口上。
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithData(buildEntryView(proxy.Runtime.Snapshot()), c)
}

// AccessLogRequest 访问日志开关
type AccessLogRequest struct {
	Enabled bool `json:"enabled"`
}

// ProxyAccessLogView 开关访问日志。
//
// 单独一个接口而不是并进入口表单：它即点即生效，
// 不需要、也不该顺带把监听重开一遍。
func (ProxyApi) ProxyAccessLogView(c *gin.Context) {
	cr := middleware.GetBindRequest[AccessLogRequest](c)
	if err := proxy.SetAccessLog(cr.Enabled); err != nil {
		res.FailWithMsg("保存失败："+err.Error(), c)
		return
	}
	if cr.Enabled {
		res.OkWithMsg("访问日志已开启", c)
		return
	}
	res.OkWithMsg("访问日志已关闭", c)
}

// ErrorPageSaveRequest 错误页设置
type ErrorPageSaveRequest struct {
	// Mode ""（默认脱敏）| "simple" | "custom"
	Mode string `json:"mode"`
	// CustomHTML 自定义整页 HTML，支持 {{status}} 与 {{message}}
	CustomHTML string `json:"customHtml"`
}

// ProxyErrorPageSaveView 保存外网错误页的显示方式
func (ProxyApi) ProxyErrorPageSaveView(c *gin.Context) {
	cr := middleware.GetBindRequest[ErrorPageSaveRequest](c)
	if err := proxy.SetErrorPage(proxymodel.ErrorPage{
		Mode:       cr.Mode,
		CustomHTML: cr.CustomHTML,
	}); err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithMsg("已保存，下一次错误响应就生效", c)
}

// ExposeAdminRequest 是否允许把管理后台通过反代挂出去
type ExposeAdminRequest struct {
	Enabled bool `json:"enabled"`
}

// ProxyExposeAdminView 开关「允许通过反代暴露管理后台」。
//
// 打开后，站点后端才可以填 127.0.0.1:<后台端口> —— 那是远程管理的唯一途径。
func (ProxyApi) ProxyExposeAdminView(c *gin.Context) {
	cr := middleware.GetBindRequest[ExposeAdminRequest](c)
	if err := proxy.SetExposeAdmin(cr.Enabled); err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	if cr.Enabled {
		res.OkWithMsg("已允许通过反代暴露管理后台。请确认审计页「永不封禁」名单里有你自己的出口 IP，否则别人故意输错几次就能把你关在门外", c)
		return
	}
	res.OkWithMsg("已关闭", c)
}
