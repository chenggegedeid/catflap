package routers

import (
	"github.com/chenggegedeid/catflap/api/webhook_api"
	"github.com/chenggegedeid/catflap/core/component"
	"github.com/chenggegedeid/catflap/middleware"
	"github.com/chenggegedeid/catflap/modules/webhook"

	"github.com/gin-gonic/gin"
)

func WebhookRouters(g *gin.RouterGroup) {
	var app = webhook_api.WebhookApi{}

	// webhook 模板：查 / 增 / 改 / 删
	g.GET(
		"webhook/templates",
		app.WebhookTemplateListView,
	)
	g.POST(
		"webhook/template/add",
		middleware.BindJsonMiddleware[webhook_api.WebhookTemplateSaveRequest],
		app.WebhookTemplateAddView,
	)
	g.PUT(
		"webhook/template/update",
		middleware.BindJsonMiddleware[webhook_api.WebhookTemplateSaveRequest],
		app.WebhookTemplateUpdateView,
	)
	g.DELETE(
		"webhook/template/delete",
		middleware.BindJsonMiddleware[webhook_api.WebhookTemplateDeleteRequest],
		app.WebhookTemplateDeleteView,
	)
}

// 把本模块的挂载交给组件注册表，routers/enter.go 不再需要列它。
func init() {
	component.MustContributeRoutes(webhook.Name, func(g component.RouterGroups) {
		WebhookRouters(g.Protected)
	})
}
