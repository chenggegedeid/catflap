package home_api

import (
	"github.com/chenggegedeid/catflap/middleware"
	"github.com/chenggegedeid/catflap/modules/home"
	"github.com/chenggegedeid/catflap/utils/res"

	"github.com/gin-gonic/gin"
)

type PageModeRequest struct {
	Mode string `json:"mode"`
}

/**
 * PageModeUpdateView 切换分类模式。
 *
 * 收敛而不是原样存：将来加第三种模式时，老配置里可能写着
 * 不认识的值 —— 统一在写入时就归一化，读取端就不用到处防御。
 */
func (HomeApi) PageModeUpdateView(c *gin.Context) {
	cr := middleware.GetBindRequest[PageModeRequest](c)

	mode := cr.Mode
	if mode != "stream" && mode != "paged" {
		mode = "stream"
	}

	err := home.Runtime.WithLock(func(cfg *home.Config) error {
		cfg.PageMode = mode
		return nil
	})
	if err != nil {
		res.FailWithMsg("保存失败", c)
		return
	}
	res.OkWithMsg("已切换", c)
}
