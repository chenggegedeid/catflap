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
	SearchHistory         []string            `json:"searchHistory"`

	Categories []home.Category `json:"categories"`
	Apps       []home.AppView  `json:"apps"`

	// LockedCategoryIDs 因为要登录而被藏起来的分组 ID。
	//
	// 前端拿它给访客渲染一个「登录查看」的占位分组——让人知道这儿有东西，
	// 但不泄露是什么。已登录时为空。
	LockedCategoryIDs []string `json:"lockedCategoryIds"`

	// Authed 这次请求带没带有效登录态。前端据此决定要不要显示"登录/退出"入口。
	// 注意它只是个展示用的提示，真正的过滤在服务端做了，不信前端传的任何东西。
	Authed bool `json:"authed"`
}

// GetHomeConfigView 整体读,含 hydrated apps
func (HomeApi) GetHomeConfigView(c *gin.Context) {
	// 导航主页是公开的，但这个接口同时给「访客浏览」和「主人编辑」两种场景用，
	// 所以要看一眼这次请求有没有登录态：有就给全量，没有就先把要求登录的分组摘掉。
	//
	// 摘的动作必须在服务端做。放前端藏的话，访客打开页面源码 / 直接调这个
	// 接口照样能看到内网服务名和端口——那等于没设防。
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
			SearchHistory:         append([]string{}, cfg.SearchHistory...),
			Categories:            append([]home.Category{}, cfg.Categories...),
			Apps:                  home.HydrateApps(cfg.Apps),
			LockedCategoryIDs:     []string{},
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
		resp.LockedCategoryIDs = make([]string, 0, len(locked))
		for id := range locked {
			resp.LockedCategoryIDs = append(resp.LockedCategoryIDs, id)
		}

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
