package routers

import (
	"github.com/chenggegedeid/catflap/api/ddns_api"
	"github.com/chenggegedeid/catflap/core/component"
	"github.com/chenggegedeid/catflap/middleware"
	"github.com/chenggegedeid/catflap/modules/ddns"

	"github.com/gin-gonic/gin"
)

func DdnsRouters(g *gin.RouterGroup) {
	var app = ddns_api.DdnsApi{}

	// 输出当前 DDNS 配置（凭证脱敏）
	g.GET(
		"ddns/config",
		app.GetDdnsConfigView,
	)

	// 更新总开关与全局同步间隔
	g.PUT(
		"ddns/settings",
		middleware.BindJsonMiddleware[ddns_api.DdnsSettingsUpdateRequest],
		app.DdnsSettingsUpdateView,
	)

	// 本机网卡列表，供「本地网卡」这个 IP 来源挑选
	g.GET(
		"ddns/interfaces",
		app.DdnsInterfaceListView,
	)

	// 支持哪些服务商、各自要哪些凭据字段。前端表单据此自动生成，
	// 新增一家服务商只加一个 register_*.go，前端零改动。
	g.GET(
		"ddns/provider/types",
		app.DdnsProviderTypesView,
	)

	// 服务商：增 / 改 / 删
	g.POST(
		"ddns/provider/add",
		middleware.BindJsonMiddleware[ddns_api.DdnsProviderAddRequest],
		app.DdnsProviderAddView,
	)
	g.PUT(
		"ddns/provider/update",
		middleware.BindJsonMiddleware[ddns_api.DdnsProviderUpdateRequest],
		app.DdnsProviderUpdateView,
	)
	g.DELETE(
		"ddns/provider/delete",
		middleware.BindJsonMiddleware[ddns_api.DdnsProviderDeleteRequest],
		app.DdnsProviderDeleteView,
	)

	// 解析记录：增 / 改 / 删
	g.POST(
		"ddns/record/add",
		middleware.BindJsonMiddleware[ddns_api.DdnsRecordAddRequest],
		app.DdnsRecordAddView,
	)
	g.PUT(
		"ddns/record/update",
		middleware.BindJsonMiddleware[ddns_api.DdnsRecordUpdateRequest],
		app.DdnsRecordUpdateView,
	)
	g.DELETE(
		"ddns/record/delete",
		middleware.BindJsonMiddleware[ddns_api.DdnsRecordDeleteRequest],
		app.DdnsRecordDeleteView,
	)

	// 立即同步（id=0 全量，否则单条）
	g.POST(
		"ddns/record/sync",
		middleware.BindJsonMiddleware[ddns_api.DdnsRecordSyncRequest],
		app.DdnsRecordSyncView,
	)
}

// 把本模块的挂载交给组件注册表，routers/enter.go 不再需要列它。
func init() {
	component.MustContributeRoutes(ddns.Name, func(g component.RouterGroups) {
		DdnsRouters(g.Protected)
	})
}
