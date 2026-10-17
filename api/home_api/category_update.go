package home_api

import (
	"errors"
	"strings"
	"time"

	"github.com/chenggegedeid/catflap/middleware"
	"github.com/chenggegedeid/catflap/modules/home"
	"github.com/chenggegedeid/catflap/utils/pwd"
	"github.com/chenggegedeid/catflap/utils/res"

	"github.com/gin-gonic/gin"
)

type CategoryUpdateRequest struct {
	ID   string `json:"id"`
	Name string `json:"name"`

	// RequireAuth 用指针区分「没传」和「传了 false」：切开关时传 false 是要真的
	// 关掉，用 bool 零值就分不清是没传还是要关。
	RequireAuth *bool `json:"requireAuth"`

	/**
	 * Visibility 三级可见性：public / login / lock。
	 *
	 * 不传就不改。传了 lock 但没给 Password（且这个分组原来没有口令）
	 * 要报错 —— 一个没有口令的 lock 分组是谁也进不去的死结。
	 */
	Visibility *string `json:"visibility"`

	/**
	 * Password 分组口令，只在 Visibility=lock 时有意义。
	 *
	 * 给了就重设（覆盖旧的）。空串表示不改。
	 * 从 lock 改成别的可见性时，哈希会被一并清掉 —— 不留没用的凭据在配置里。
	 */
	Password string `json:"password,omitempty"`
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
				// 先定可见性，再定口令 —— 顺序反了会在刚设成 public 之后
				// 又把口令写回去
				vis := home.NormalizeVisibility(cfg.Categories[i].Visibility)
				if cr.Visibility != nil {
					vis = home.NormalizeVisibility(*cr.Visibility)
				}
				if cr.RequireAuth != nil && cr.Visibility == nil {
					// 老字段兼容：老前端还在用 requireAuth 开关
					if *cr.RequireAuth {
						vis = home.VisibilityLogin
					} else {
						vis = home.VisibilityPublic
					}
				}
				if vis == home.VisibilityLock && cfg.Categories[i].PasswordHash == "" && strings.TrimSpace(cr.Password) == "" {
					return errors.New("上锁的分组必须先设一个口令，否则谁也进不去")
				}
				cfg.Categories[i].Visibility = vis
				// 老字段跟着走，让旧版前端读到的状态和新字段一致
				cfg.Categories[i].RequireAuth = vis != home.VisibilityPublic

				if pw := strings.TrimSpace(cr.Password); pw != "" {
					hash, err := pwd.HashPassword(pw)
					if err != nil {
						return err
					}
					cfg.Categories[i].PasswordHash = hash
				} else if vis != home.VisibilityLock {
					// 不再上锁了就把口令清掉，不留没用的哈希
					cfg.Categories[i].PasswordHash = ""
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
