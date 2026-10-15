package home_api

import (
	"github.com/chenggegedeid/catflap/middleware"
	"github.com/chenggegedeid/catflap/modules/home"
	"github.com/chenggegedeid/catflap/utils/res"

	"github.com/gin-gonic/gin"
)

type HomeConfigViewResponse struct {
	Title    string `json:"title"`
	ShowTime bool   `json:"showTime"`

	Wallpaper home.Wallpaper `json:"wallpaper"`

	LayoutMode    string `json:"layoutMode"`
	NetworkPrefer string `json:"networkPrefer"`

	DefaultSearchEngineID string              `json:"defaultSearchEngineId"`
	SearchEngines         []home.SearchEngine `json:"searchEngines"`

	Categories []home.Category `json:"categories"`
	Apps       []home.AppView  `json:"apps"`

	// Authed 这次请求带没带有效登录态。前端据此决定右上角显示「登录」还是
	// 「设置 / 后台 / 退出」。
	//
	// 注意它只是展示用：真正的过滤在服务端做了（见下），不信前端传的任何东西。
	Authed bool `json:"authed"`
}

// GetHomeConfigView 整体读,含 hydrated apps
func (HomeApi) GetHomeConfigView(c *gin.Context) {
	// 导航主页是公开的，但这个接口同时给「访客浏览」和「主人编辑」两种场景用，
	// 所以要看一眼这次请求有没有登录态：有就给全量，没有就先把要求登录的分组摘掉。
	//
	// 摘的动作必须在服务端做，而且**摘干净**：
	// 不回 lockedCategoryIds、不留占位 —— 访客拿到的 JSON 里就不存在那些分组，
	// 从响应里数不出"您藏了几个分组"。占位数本身就是信息（有几个东西值得惦记）。
	authed := middleware.IsAuthed(c)

	var resp HomeConfigViewResponse
	home.Runtime.Read(func(cfg *home.Config) {
		// 注意:必须用 []T{} 作为 base,append 空切片到 []T(nil) 仍返回 nil,
		// JSON 会序列化成 null,前端拿到 null.length 直接崩
		resp = HomeConfigViewResponse{
			Title:                 cfg.Title,
			ShowTime:              cfg.ShowTime,
			Wallpaper:             cfg.Wallpaper,
			LayoutMode:            cfg.LayoutMode,
			NetworkPrefer:         cfg.NetworkPrefer,
			DefaultSearchEngineID: cfg.DefaultSearchEngineID,
			SearchEngines:         append([]home.SearchEngine{}, cfg.SearchEngines...),
			Categories:            append([]home.Category{}, cfg.Categories...),
			Apps:                  home.HydrateApps(cfg.Apps),
			Authed:                authed,
		}

		if authed {
			return
		}

		// 未登录：摘掉要求登录的分组，以及挂在这些分组下的应用。
		locked := make(map[string]bool)
		kept := make([]home.Category, 0, len(resp.Categories))
		for _, cat := range resp.Categories {
			if cat.RequireAuth {
				locked[cat.ID] = true
				continue
			}
			kept = append(kept, cat)
		}
		if len(locked) == 0 {
			return
		}

		resp.Categories = kept

		apps := make([]home.AppView, 0, len(resp.Apps))
		for _, a := range resp.Apps {
			if locked[a.CategoryID] {
				continue
			}
			apps = append(apps, a)
		}
		resp.Apps = apps
	})
	res.OkWithData(resp, c)
}
