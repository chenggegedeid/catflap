package home_api

import (
	"github.com/chenggegedeid/catflap/middleware"
	"github.com/chenggegedeid/catflap/modules/home"
	"github.com/chenggegedeid/catflap/utils/res"

	"github.com/gin-gonic/gin"
)

type AppMoveRequest struct {
	ID         string `json:"id"`
	CategoryID string `json:"categoryId"`
}

/**
 * AppMoveView 把应用移到别的分组。
 *
 * 顺序的处理是关键：移到新分组后要**排在那一组的末尾**，
 * 否则它会带着旧分组的 order 插进去，落到中间某个位置 ——
 * 用户拖到"常用"分组，结果它出现在常用组的第二个，很莫名其妙。
 *
 * 做法：给它在目标组内取 max(order)+1。
 */
func (HomeApi) AppMoveView(c *gin.Context) {
	cr := middleware.GetBindRequest[AppMoveRequest](c)

	err := home.Runtime.WithLock(func(cfg *home.Config) error {
		/**
		 * 这里先自己校验"目标分组存在"，再调纯函数。
		 *
		 * 为什么不放进 MoveAppToCategory：那个函数只拿得到 Apps，
		 * 而一个**空分组**在 Apps 里没有任何成员 —— 它分不清
		 * "目标组是空的"和"压根没这个组"。分组列表只有这里查得到。
		 * 校验放对地方，才不会出现"应用被移进不存在的分组后消失"这种事故。
		 */
		catExists := false
		for _, cat := range cfg.Categories {
			if cat.ID == cr.CategoryID {
				catExists = true
				break
			}
		}
		if !catExists {
			return home.ErrNotFound
		}

		appExists := false
		for _, a := range cfg.Apps {
			if a.ID == cr.ID {
				appExists = true
				break
			}
		}
		if !appExists {
			return home.ErrNotFound
		}

		cfg.Apps = home.MoveAppToCategory(cfg.Apps, cr.ID, cr.CategoryID)
		return nil
	})
	if err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithMsg("已移动", c)
}
