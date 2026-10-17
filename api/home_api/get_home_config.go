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

	/**
	 * HiddenCategoryCount 因为权限被藏起来的分组**数量**。
	 *
	 * 只给数量，不给名单、不给名字 —— 让访客知道"这儿有个上锁的分组"
	 * 本身就是信息，等于告诉他值得来试。前端据此渲染一个不带名字的锁形占位。
	 */
	HiddenCategoryCount int `json:"hiddenCategoryCount,omitempty"`

	// PageMode 分类切换方式：stream（一条长流）或 paged（一屏一组）。
	// 空串按 stream 处理 —— 老配置没有这个字段。
	PageMode string `json:"pageMode,omitempty"`

	// Dock 底部托盘的应用（按配置顺序，已过滤掉不存在的 id）。
	// 访客也拿得到 —— 托盘里放什么本来就该跟着权限走，见下。
	Dock []home.AppView `json:"dock"`

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
			LayoutMode:            cfg.LegacyLayoutMode, // TODO(M4)：新前端不再需要，删
			NetworkPrefer:         cfg.NetworkPrefer,
			DefaultSearchEngineID: cfg.DefaultSearchEngineID,
			SearchEngines:         append([]home.SearchEngine{}, cfg.SearchEngines...),
			Categories:            append([]home.Category{}, cfg.Categories...),
			Apps:                  home.HydrateApps(cfg.Apps),
			Authed:                authed,
			PageMode:              normalizePageMode(cfg.PageMode),
		}

		// 托盘跟着"这次能看到的应用"走：访客的托盘里不该出现私密分组的应用。
		// 做法是拿过滤后的 resp.Apps 去匹配 DockIDs —— 不用单独再算一遍权限。
		resp.Dock = hydrateDock(cfg.DockIDs, resp.Apps)

		if authed {
			resp.Sites = proxySiteOptions()

			/**
			 * 已登录：lock 分组还要再看**这次会话有没有解锁过**。
			 *
			 * 登录态挡不住"家人打开你电脑"这个场景 —— 你登着，
			 * 别人坐过来就能看。lock 是为此单独加的一道。
			 */
			unlocked := home.VerifyUnlockToken(home.ReadUnlockToken(c))
			locked := make(map[string]bool)
			for _, cat := range resp.Categories {
				if cat.Visibility == home.VisibilityLock && !unlocked[cat.ID] {
					locked[cat.ID] = true
				}
			}
			if len(locked) > 0 {
				apps := make([]home.AppView, 0, len(resp.Apps))
				for _, a := range resp.Apps {
					if !locked[a.CategoryID] {
						apps = append(apps, a)
					}
				}
				resp.Apps = apps
				// 主人在场时给出数量，让他知道这儿还有锁着的东西（他自己的分组，不算泄密）
				resp.HiddenCategoryCount = len(locked)
			}
			return
		}
		resp.Sites = nil

		/**
		 * 未登录：只留 public 分组。
		 *
		 * login 和 lock 都不下发（lock 连口令提示都不给 —— 让访客知道
		 * "这儿有个上锁的分组"本身就是信息，等于告诉他值得来试）。
		 *
		 * 顺带把内网地址清掉：见下面的 stripLAN。
		 */
		kept := make([]home.Category, 0, len(resp.Categories))
		for _, cat := range resp.Categories {
			if cat.Visibility == home.VisibilityPublic {
				kept = append(kept, cat)
			}
		}
		if len(kept) != len(resp.Categories) {
			// 只回**数量**，不回名单、也不回占位 —— "你藏了几个"同样是信息。
			// 前端据此渲染一个不带名字的锁形占位，让主人自己知道这儿有东西。
			resp.HiddenCategoryCount = len(resp.Categories) - len(kept)
			resp.Categories = kept

			allowed := make(map[string]bool, len(kept))
			for _, c := range kept {
				allowed[c.ID] = true
			}
			apps := make([]home.AppView, 0, len(resp.Apps))
			for _, a := range resp.Apps {
				if allowed[a.CategoryID] {
					apps = append(apps, a)
				}
			}
			resp.Apps = apps
		}

		// 访客拿不到内网地址。公网入口对外网访客本来就用不上，
		// 给出去只是白送一份内网拓扑。
		stripLAN(resp.Apps)

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

// stripLAN 把应用的内网入口清掉（只作用于本次下发的视图，不动配置）。
//
// 为什么只在未登录时做：主人自己看的时候，内网地址是他要用的东西。
// 而外网访客既连不上（不在同一个局域网），也不该知道 —— 那是内网拓扑。
func stripLAN(apps []home.AppView) {
	for i := range apps {
		apps[i].Addresses.LAN = ""
	}
}

/**
 * hydrateDock 把托盘 id 列表展开成应用视图。
 *
 * 关键点：**只从 visible 里取**。visible 是这次请求过滤后的应用列表
 * （访客看不到私密分组的），所以托盘的可见性自动和分组权限一致，
 * 不需要在这里重算一遍 —— 权限判断只有一处，就不会有第二种口径。
 *
 * 应用被删了、或者没权限看，就直接跳过（前端拿到的托盘是干净的）。
 */
func hydrateDock(ids []string, visible []home.AppView) []home.AppView {
	if len(ids) == 0 {
		return []home.AppView{}
	}
	byID := make(map[string]home.AppView, len(visible))
	for _, a := range visible {
		byID[a.ID] = a
	}
	out := make([]home.AppView, 0, len(ids))
	for _, id := range ids {
		if a, ok := byID[id]; ok {
			out = append(out, a)
		}
	}
	return out
}

/**
 * normalizePageMode 空串按 paged 处理。
 *
 * 老配置没有这个字段。默认值选 paged 而不是 stream 的原因：
 * paged 下点击分组是**内容替换**，永远有即时反馈；
 * stream 下点击是滚动，内容不足一屏时毫无反应 —— 用户会以为功能坏了。
 * 宁可让喜欢流式的人去设置里切一下，也不能让默认路径看起来像坏的。
 */
func normalizePageMode(v string) string {
	if v == "stream" {
		return "stream"
	}
	return "paged"
}
