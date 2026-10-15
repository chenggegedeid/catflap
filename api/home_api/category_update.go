package home_api

import (
	"github.com/chenggegedeid/catflap/middleware"
	"github.com/chenggegedeid/catflap/modules/home"
	"github.com/chenggegedeid/catflap/utils/res"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type CategoryUpdateRequest struct {
	ID   string `json:"id"`
	Name string `json:"name"`

	// RequireAuth 用指针区分「没传」和「传了 false」：切开关时传 false 是要真的
	// 关掉，用 bool 零值就分不清是没传还是要关。
	RequireAuth *bool `json:"requireAuth"`
}

func (HomeApi) CategoryUpdateView(c *gin.Context) {
	cr := middleware.GetBindRequest[CategoryUpdateRequest](c)
	if cr.ID == "" {
		res.FailWithMsg("id 不能为空", c)
		return
	}
	// 只改开关时可以不带 name；但要改名时不能给个空白名字
	if cr.RequireAuth == nil && strings.TrimSpace(cr.Name) == "" {
		res.FailWithMsg("name 不能为空", c)
		return
	}

	err := home.Runtime.WithLock(func(cfg *home.Config) error {
		for i := range cfg.Categories {
			if cfg.Categories[i].ID == cr.ID {
				if cr.RequireAuth != nil {
					cfg.Categories[i].RequireAuth = *cr.RequireAuth
				}
				if strings.TrimSpace(cr.Name) != "" {
					cfg.Categories[i].Name = cr.Name
				}
				cfg.Categories[i].UpdatedAt = time.Now()
				return nil
			}
		}
		return home.ErrNotFound
	})
	if err == home.ErrNotFound {
		res.FailWithMsg("分类不存在", c)
		return
	}
	if err != nil {
		res.FailWithMsg("保存配置失败", c)
		return
	}
	res.OkWithMsg("更新成功", c)
}
