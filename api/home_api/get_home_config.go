package home_api

import (
	"github.com/chenggegedeid/catflap/middleware"
	"github.com/chenggegedeid/catflap/modules/home"
	"github.com/chenggegedeid/catflap/modules/proxy"
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

	// Sites 反代站点里标记了「显示在主页」的那些。**只在已登录时给** ——
	// 站点信息（有哪些服务）不该让访客看到。
	//
	// 主页条目本身是手填的，这里只是给"添加条目"时提供可选来源：
	// 选一个站点，条目就指向它，不用手打地址。
	Sites []SiteOption `json:"sites,omitempty"`

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
			Sites:                 []SiteOption{},
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

		// 站点列表只给登录的人（见 Sites 字段的注释）
		if authed {
			resp.Sites = proxySiteOptions()
			return
		}
		resp.Sites = nil

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

// SiteOption 一个可以在主页上放出来的站点
type SiteOption struct {
	ID      uint   `json:"id"`
	Host    string `json:"host"`
	Name    string `json:"name"`
	HTTPS   bool   `json:"https"`
	Enabled bool   `json:"enabled"`
}

// proxySiteOptions 从反代配置里取「要在主页显示」的站点。
//
// **在 api 层做**：home 不 import proxy（那个模块有自己的监听与生命周期，
// 门户页不该被它拖住），两边都只在这层碰头。
func proxySiteOptions() []SiteOption {
	out := []SiteOption{}
	for _, s := range proxy.Runtime.Config.Sites {
		if !s.ShowOnHome {
			continue
		}
		// 停用的站点**照样列出**，只是标一下停用了。
		//
		// 不然会出现这种困惑：用户停用站点（比如临时维护），标记还在，
		// 但主页上看不见了 —— 重新启用后也不会自动回来，因为用户以为
		// "标记丢了"又去点了一次，反而取消了。
		//
		// 真要访问时主页那边再判断可不可达（水合阶段会给出地址）。
		if len(s.Hosts) == 0 {
			continue
		}
		out = append(out, SiteOption{
			ID:      s.ID,
			Host:    s.Hosts[0],
			Name:    s.Description,
			HTTPS:   s.HTTPS,
			Enabled: s.Enabled,
		})
	}
	return out
}
