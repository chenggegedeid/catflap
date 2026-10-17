package home_api

import (
	"sort"

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
		index := make(map[string]int, len(cr.IDs))
		for i, id := range cr.IDs {
			index[id] = i
		}

		// 分成「点名的」和「没点名的」两堆，各自按新顺序 / 原顺序排好，再拼起来。
		// 拼完一把重新编号 1..N —— 编号连续，前端和迁移逻辑都不用处理空洞。
		listed := make([]home.App, 0, len(cr.IDs))
		rest := make([]home.App, 0)
		for _, a := range cfg.Apps {
			if _, ok := index[a.ID]; ok {
				listed = append(listed, a)
			} else {
				rest = append(rest, a)
			}
		}
		sort.SliceStable(listed, func(i, j int) bool { return index[listed[i].ID] < index[listed[j].ID] })
		sort.SliceStable(rest, func(i, j int) bool { return rest[i].Order < rest[j].Order })

		out := append(listed, rest...)
		for i := range out {
			out[i].Order = i + 1
		}
		cfg.Apps = out

		// 旧契约兼容：旧版导航页还读这两个字段
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
	if err != nil {
		res.FailWithMsg("保存配置失败", c)
		return
	}
	res.OkWithMsg("更新成功", c)
}
