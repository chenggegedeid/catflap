package routers

import (
	"github.com/chenggegedeid/catflap/api/cert_api"
	"github.com/chenggegedeid/catflap/core/component"
	"github.com/chenggegedeid/catflap/middleware"
	"github.com/chenggegedeid/catflap/modules/cert"

	"github.com/gin-gonic/gin"
)

func CertRouters(g *gin.RouterGroup) {
	var app = cert_api.CertApi{}

	// 证书列表（含有效期、剩余天数、最近一次错误）
	g.GET(
		"cert/list",
		app.CertListView,
	)

	// 可用于 DNS-01 的服务商（复用 DDNS 已配好的凭证）
	g.GET(
		"cert/providers",
		app.CertProvidersView,
	)

	// 增 / 改 / 删
	g.POST(
		"cert/create",
		middleware.BindJsonMiddleware[cert_api.CertSaveRequest],
		app.CertAddView,
	)
	g.PUT(
		"cert/update",
		middleware.BindJsonMiddleware[cert_api.CertSaveRequest],
		app.CertUpdateView,
	)
	g.DELETE(
		"cert/remove",
		middleware.BindJsonMiddleware[cert_api.CertDeleteRequest],
		app.CertDeleteView,
	)

	// 上传 PEM：JSON 粘贴或 multipart 选文件，由处理函数按 Content-Type 分流
	g.POST(
		"cert/upload",
		app.CertUploadView,
	)

	// 立即签发/续期（异步）
	g.POST(
		"cert/issue",
		middleware.BindJsonMiddleware[cert_api.CertIssueRequest],
		app.CertIssueView,
	)
}

// 把本模块的挂载交给组件注册表，routers/enter.go 不再需要列它。
func init() {
	component.MustContributeRoutes(cert.Name, func(g component.RouterGroups) {
		CertRouters(g.Protected)
	})
}
