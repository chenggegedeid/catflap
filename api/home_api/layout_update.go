package home_api

import (
	"github.com/chenggegedeid/catflap/middleware"
	"github.com/chenggegedeid/catflap/utils/res"

	"github.com/gin-gonic/gin"
)

// LayoutUpdateRequest 保留这个接口只为兼容旧版前端。
//
// 新架构只有一种布局：**流式网格**（列数随窗口宽度变，往下排）。
// 左右/上下/自由翻页那几种模式已经没有了 —— 它们的存在前提是固定 8×5 分页，
// 而跨格卡片（2×2、2×4）和固定分页是冲突的。
//
// TODO(M4)：新前端落地后把这个接口和路由一起删掉。
type LayoutUpdateRequest struct {
	LayoutMode string `json:"layoutMode"`
}

func (HomeApi) LayoutUpdateView(c *gin.Context) {
	_ = middleware.GetBindRequest[LayoutUpdateRequest](c)
	res.OkWithMsg("新版只有流式网格，布局模式已不再需要", c)
}
