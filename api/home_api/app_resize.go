package home_api

import (
	"github.com/chenggegedeid/catflap/middleware"
	"github.com/chenggegedeid/catflap/modules/home"
	"github.com/chenggegedeid/catflap/utils/res"

	"github.com/gin-gonic/gin"
)

// AppResizeRequest 改卡片尺寸（右键菜单「修改大小」）。
//
// 尺寸是**宽×高**的字符串（1x1 / 1x2 / 2x1 / 2x2 / 2x4），
// 合法值在 modules/home 里校验 —— 校验放模块层而不是接口层，
// 是因为将来可能有别的入口（批量、导入）也要走这道。
type AppResizeRequest struct {
	ID   string `json:"id"`
	Size string `json:"size"`
}

func (HomeApi) AppResizeView(c *gin.Context) {
	cr := middleware.GetBindRequest[AppResizeRequest](c)
	err := home.Runtime.WithLock(func(cfg *home.Config) error {
		for i := range cfg.Apps {
			if cfg.Apps[i].ID != cr.ID {
				continue
			}
			return home.SetAppSize(&cfg.Apps[i], cr.Size)
		}
		return home.ErrNotFound
	})
	if err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithMsg("已调整", c)
}
