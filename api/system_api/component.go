package system_api

import (
	"github.com/chenggegedeid/catflap/core/component"
	"github.com/chenggegedeid/catflap/middleware"
	"github.com/chenggegedeid/catflap/utils/res"

	"github.com/gin-gonic/gin"
)

// ComponentListView 功能管理页要的全部信息。
//
// 为什么把健康状态和元信息分成两块返回：元信息来自注册表（几乎不变，是
// 「有哪些功能、能不能开关」），健康状态要现算（会变，是「现在好不好」）。
// 分开之后前端可以只刷新后者，也看得清哪些字段是配置、哪些是运行态。
type ComponentListView struct {
	Components []component.Descriptor      `json:"components"`
	Health     map[string]component.Health `json:"health,omitempty"`
	// Disabled 当前被关掉的组件名。冗余于 Components[].enabled，但让前端
	// 一眼能读到「这台机器被改过什么」，不用自己遍历筛。
	Disabled []string `json:"disabled"`
}

// ListComponents 列出全部组件及其开关、能力与健康状态。
//
// 方法名用动词开头、不以 View 结尾，不是为了标新立异：
//   - 响应类型已经叫 ComponentListView 了，方法再叫同一个名字，同一文件里
//     两个同名标识符会让人先分清楚眼前这个是类型还是函数（Go 允许，方法名
//     在方法集里不占包作用域，但没必要付这个理解成本）；
//   - 本文件里 GetVersion 本来就是动词开头，「XxxView」不是必须遵守的规矩。
func (SystemApi) ListComponents(c *gin.Context) {
	res.OkWithData(ComponentListView{
		Components: component.Descriptors(),
		Health:     component.Healths(),
		Disabled:   component.DisabledNames(),
	}, c)
}

// ComponentEnableRequest 开关一个组件
type ComponentEnableRequest struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

// SetComponentEnabled 开关一个组件。
//
// 立即生效：路由闸门每次请求都会读当前状态，不需要重启。
// 落盘在 config/components.json，重启后保持。
//
// 失败（组件名不存在、或该组件不支持开关，如 auth / home / system）走的是
// 项目统一的业务错误约定：HTTP 200 + body 里 code != 0 —— 与其它接口一致，
// 前端才会按同一套逻辑提示。这里刻意不回 4xx：本项目的 http 状态码只用于
// 「传输层/鉴权层」的语义（见 res.FailUnauthorized / FailForbidden）。
func (SystemApi) SetComponentEnabled(c *gin.Context) {
	cr := middleware.GetBindRequest[ComponentEnableRequest](c)

	if err := component.SetEnabled(cr.Name, cr.Enabled); err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithMsg("已保存", c)
}
