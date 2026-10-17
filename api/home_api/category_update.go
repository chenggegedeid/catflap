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

	// Icon 分组图标。可以是上传/抓取的路径，也可以是内置图标库的 `lib:<名字>`
	Icon *string `json:"icon,omitempty"`
}

func (HomeApi) CategoryUpdateView(c *gin.Context) {
	cr := middleware.GetBindRequest[CategoryUpdateRequest](c)
	if cr.ID == "" {
		res.FailWithMsg("id 不能为空", c)
		return
	}
	/**
	 * 至少要有一样东西要改：改名、改可见性、或设口令。
	 *
	 * 只改可见性时不必带 name —— 逼人把名字重发一遍，
	 * 既啰嗦又可能因为顺手一改把名字也改了。
	 */
	/**
	 * 每加一个可改字段，都要同步到这里。
	 *
	 * 之前加了 Icon 忘了加进这行 —— 于是"只改图标"的请求被判成"没有要修改的内容"，
	 * 界面上就是点图标没反应。这类漏同步不会有编译错误，只能靠测试/实测发现。
	 * Icon 用指针判 nil 而不是判空串：**传空串是要把图标清掉**，那是有效的修改。
	 */
	hasWork :=
		strings.TrimSpace(cr.Name) != "" ||
			cr.Visibility != nil ||
			cr.RequireAuth != nil ||
			cr.Icon != nil ||
			strings.TrimSpace(cr.Password) != ""
	if !hasWork {
		res.FailWithMsg("没有要修改的内容", c)
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

				if cr.Icon != nil {
					cfg.Categories[i].Icon = strings.TrimSpace(*cr.Icon)
				}

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
