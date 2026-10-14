package routers

import (
	"github.com/chenggegedeid/catflap/api/task_api"
	"github.com/chenggegedeid/catflap/core/component"
	"github.com/chenggegedeid/catflap/middleware"
	"github.com/chenggegedeid/catflap/modules/task"

	"github.com/gin-gonic/gin"
)

func TaskRouters(g *gin.RouterGroup) {
	var app = task_api.TaskApi{}

	g.GET(
		"task/tasks",
		app.TaskListView,
	)
	g.POST(
		"task/task/add",
		middleware.BindJsonMiddleware[task_api.TaskSaveRequest],
		app.TaskAddView,
	)
	g.PUT(
		"task/task/update",
		middleware.BindJsonMiddleware[task_api.TaskSaveRequest],
		app.TaskUpdateView,
	)
	g.DELETE(
		"task/task/delete",
		middleware.BindJsonMiddleware[task_api.TaskDeleteRequest],
		app.TaskDeleteView,
	)
	// 手动跑一次。不写配置，所以用 POST
	g.POST(
		"task/task/run",
		middleware.BindJsonMiddleware[task_api.TaskRunRequest],
		app.TaskRunView,
	)
}

// 把本模块的挂载交给组件注册表，routers/enter.go 不再需要列它。
func init() {
	component.MustContributeRoutes(task.Name, func(g component.RouterGroups) {
		TaskRouters(g.Protected)
	})
}
