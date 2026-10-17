package home_api

import (
	"github.com/chenggegedeid/catflap/middleware"
	"github.com/chenggegedeid/catflap/modules/home"
	"github.com/chenggegedeid/catflap/utils/res"

	"github.com/gin-gonic/gin"
)

// AppReorderRequest 重排应用。
//
// ids 是**新的顺序**：列表里第 i 个就排到第 i 位。
// 没出现在 ids 里的应用不会丢 —— 它们按自己原来的相对顺序接到末尾。
// 前端少传几个顶多是把那几个排到最后，不会有人消失。
type AppReorderRequest struct {
	IDs []string `json:"ids"`
	// 旧契约：paged / scroll。新架构不区分布局模式，收到就顺手把旧字段也写上，
	// 好让旧版导航页在 M4 之前还能显示。TODO(M4)：删
	Mode       string `json:"mode,omitempty"`
	CategoryID string `json:"categoryId,omitempty"`
}

func (HomeApi) AppReorderView(c *gin.Context) {
	cr := middleware.GetBindRequest[AppReorderRequest](c)
	if len(cr.IDs) == 0 {
		res.FailWithMsg("ids 不能为空", c)
		return
	}

	err := home.Runtime.WithLock(func(cfg *home.Config) error {
		cfg.Apps = home.ReorderWithinGroups(cfg.Apps, cr.IDs)
		return nil
	})

	// 旧契约兼容：旧版导航页还读这两个字段
	if err == nil && (cr.Mode == "paged" || cr.Mode == "scroll") {
		_ = home.Runtime.WithLock(func(cfg *home.Config) error {
			switch cr.Mode {
			case "paged":
				for i := range cfg.Apps {
					cfg.Apps[i].LegacyPagedOrder = cfg.Apps[i].Order
				}
			case "scroll":
				pos := 0
				for i := range cfg.Apps {
					if cfg.Apps[i].CategoryID != cr.CategoryID {
						continue
					}
					pos++
					cfg.Apps[i].LegacyScrollOrder = pos
				}
			}
			return nil
		})
	}
	if err != nil {
		res.FailWithMsg("保存配置失败", c)
		return
	}
	res.OkWithMsg("更新成功", c)
}
