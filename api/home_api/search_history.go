package home_api

import (
	"github.com/chenggegedeid/catflap/utils/res"

	"github.com/gin-gonic/gin"
)

// 搜索历史改为**存在浏览器本地**（localStorage），服务端不再存也不再下发。
//
// 起因：导航主页是公开页面，而这个接口也是公开的 —— 任何人打开
// `GET /api/home/search-history` 就能拿到管理员搜过什么。"搜过什么"是很私人的
// 东西（可能包含内网主机名、业务关键词），而且它本来也不需要跨设备同步：
// 搜索历史就是"这台设备上我刚才搜过啥"。
//
// 放在本地还有个额外好处：不再有人往服务器上写，也就没有"忘记清理"的问题。
//
// 这三个接口保留为**明确的废弃端点**而不是直接删掉：老版本前端（浏览器里
// 缓存的 dist）还会调它们，直接 404 会在控制台刷错误。
const searchHistoryDeprecatedMsg = "搜索历史已改为保存在浏览器本地，请刷新页面"

// SearchHistoryGetView 已废弃：不再返回任何历史
func (HomeApi) SearchHistoryGetView(c *gin.Context) {
	// 返回空列表而不是报错：老前端拿到空列表会正常渲染"没有历史"，
	// 比抛错让它弹个失败提示要好。
	res.OkWithData([]string{}, c)
}

// SearchHistoryAddView 已废弃：不再在服务端记录
func (HomeApi) SearchHistoryAddView(c *gin.Context) {
	res.FailWithMsg(searchHistoryDeprecatedMsg, c)
}

// SearchHistoryClearView 已废弃
func (HomeApi) SearchHistoryClearView(c *gin.Context) {
	res.FailWithMsg(searchHistoryDeprecatedMsg, c)
}
