package home_api

import (
	"github.com/chenggegedeid/catflap/middleware"
	"github.com/chenggegedeid/catflap/modules/home"
	"github.com/chenggegedeid/catflap/utils/res"

	"github.com/gin-gonic/gin"
)

type DockUpdateRequest struct {
	// IDs 托盘的完整内容（按显示顺序）。空数组 = 清空托盘
	IDs []string `json:"ids"`
}

const maxDockItems = 16

/**
 * DockUpdateView 保存底部托盘。
 *
 * 传的是**完整列表**而不是"加一个/删一个"：增删改都是同一件事（"托盘现在长这样"），
 * 一个接口搞定。用 add/remove 两个接口的话，并发点两下就会出现
 * "加和删交错"的结果，还要靠前端祈祷顺序。
 *
 * 会过滤掉不存在的应用 id、去重、限量 —— 这些都放在服务端做：
 * 前端可能拿着旧数据（应用刚被删了）发过来，服务端不该照单全收。
 */
func (HomeApi) DockUpdateView(c *gin.Context) {
	cr := middleware.GetBindRequest[DockUpdateRequest](c)

	err := home.Runtime.WithLock(func(cfg *home.Config) error {
		exists := make(map[string]bool, len(cfg.Apps))
		for _, a := range cfg.Apps {
			exists[a.ID] = true
		}

		seen := make(map[string]bool, len(cr.IDs))
		out := make([]string, 0, len(cr.IDs))
		for _, id := range cr.IDs {
			if !exists[id] || seen[id] {
				continue
			}
			seen[id] = true
			out = append(out, id)
			if len(out) >= maxDockItems {
				break
			}
		}
		cfg.DockIDs = out
		return nil
	})
	if err != nil {
		res.FailWithMsg("保存托盘失败", c)
		return
	}
	res.OkWithMsg("已保存", c)
}
