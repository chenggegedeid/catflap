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

// 仅支持 type=static (常用网站)。stun 类型由 stun 模块联动创建
type AppAddRequest struct {
	Name       string            `json:"name"`
	Icon       string            `json:"icon"`
	Color      string            `json:"color"`
	CategoryID string            `json:"categoryId"`
	Addresses  home.AppAddresses `json:"addresses"`
	// Order 排在第几个（1 开始）。0 或未提供时排到最后
	Order int `json:"order,omitempty"`
	// 翻页布局下的绝对槽位（旧契约）。TODO(M4)：删
	PagedOrder int `json:"pagedOrder,omitempty"`
}

func (HomeApi) AppAddView(c *gin.Context) {
	cr := middleware.GetBindRequest[AppAddRequest](c)
	if strings.TrimSpace(cr.Name) == "" {
		res.FailWithMsg("name 不能为空", c)
		return
	}
	if strings.TrimSpace(cr.Addresses.WANv4) == "" &&
		strings.TrimSpace(cr.Addresses.WANv6) == "" &&
		strings.TrimSpace(cr.Addresses.LAN) == "" {
		res.FailWithMsg("至少填写一个地址", c)
		return
	}

	var created home.App
	err := home.Runtime.WithLock(func(cfg *home.Config) error {
		// 校验分类存在（空 = 未分类，允许）
		if cr.CategoryID != "" {
			ok := false
			for _, cat := range cfg.Categories {
				if cat.ID == cr.CategoryID {
					ok = true
					break
				}
			}
			if !ok {
				return home.ErrNotFound
			}
		}
		now := time.Now()
		addrs := cr.Addresses
		maxOrder, maxPaged, maxScroll := 0, 0, 0
		for _, a := range cfg.Apps {
			if a.Order > maxOrder {
				maxOrder = a.Order
			}
			if a.LegacyPagedOrder > maxPaged {
				maxPaged = a.LegacyPagedOrder
			}
			if a.CategoryID == cr.CategoryID && a.LegacyScrollOrder > maxScroll {
				maxScroll = a.LegacyScrollOrder
			}
		}
		// 前端可以指定插入位置；没指定就排到最后。
		// cr.Order 是 1 开始的位置（第几个），不是排序键。
		order := cr.Order
		if order <= 0 || order > maxOrder+1 {
			order = maxOrder + 1
		} else {
			// 插到中间：把后面的整体后移一位，保持 1..N 连续
			for i := range cfg.Apps {
				if cfg.Apps[i].Order >= order {
					cfg.Apps[i].Order++
				}
			}
		}
		pagedOrder := cr.PagedOrder
		if pagedOrder <= 0 {
			pagedOrder = maxPaged + 1
		}
		created = home.App{
			ID:                fmt.Sprintf("app-%d", now.UnixNano()),
			Name:              cr.Name,
			Icon:              cr.Icon,
			Color:             cr.Color,
			CategoryID:        cr.CategoryID,
			Order:             order,
			Size:              home.DefaultCardSize(),
			LegacyPagedOrder:  pagedOrder,
			LegacyScrollOrder: maxScroll + 1,
			Type:              "static",
			Addresses:         &addrs,
			CreatedAt:         now,
			UpdatedAt:         now,
		}
		cfg.Apps = append(cfg.Apps, created)
		return nil
	})
	if err == home.ErrNotFound {
		res.FailWithMsg("分类不存在", c)
		return
	}
	if err != nil {
		res.FailWithMsg("保存配置失败", c)
		return
	}
	res.OkWithData(created, c)
}
