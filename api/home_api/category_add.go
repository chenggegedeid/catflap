package home_api

import (
	"fmt"
	"github.com/chenggegedeid/catflap/middleware"
	"github.com/chenggegedeid/catflap/modules/home"
	"github.com/chenggegedeid/catflap/utils/res"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type CategoryAddRequest struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (HomeApi) CategoryAddView(c *gin.Context) {
	cr := middleware.GetBindRequest[CategoryAddRequest](c)
	name := strings.TrimSpace(cr.Name)
	if name == "" {
		res.FailWithMsg("name 不能为空", c)
		return
	}

	var created home.Category
	err := home.Runtime.WithLock(func(cfg *home.Config) error {
		id := cr.ID
		if id == "" {
			id = fmt.Sprintf("cat-%d", time.Now().UnixNano())
		}
		for _, c := range cfg.Categories {
			if c.ID == id {
				return home.ErrConflict
			}
		}
		maxOrder := 0
		for _, c := range cfg.Categories {
			if c.Order > maxOrder {
				maxOrder = c.Order
			}
		}
		now := time.Now()
		/**
		 * Visibility **必须显式写**，不能留空。
		 *
		 * 留空的话：后端过滤访客用的是 `Visibility == "public"`，空串不等于
		 * "public" —— 于是"新建的分组"和"迁移过的分组"在数据上不是一回事，
		 * 靠迁移那一次兜底根本兜不住（迁移只在启动时跑，新建的永远补不上）。
		 *
		 * 原则：**写入时就归一化**，别指望读取端到处防御。
		 */
		created = home.Category{
			ID:         id,
			Name:       name,
			Order:      maxOrder + 1,
			Visibility: home.VisibilityPublic,
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		cfg.Categories = append(cfg.Categories, created)
		return nil
	})
	if err == home.ErrConflict {
		res.FailWithMsg("ID 已存在", c)
		return
	}
	if err != nil {
		res.FailWithMsg("保存配置失败", c)
		return
	}
	res.OkWithData(created, c)
}
