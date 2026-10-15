package home

import "time"

const SearchHistoryMax = 24

type Config struct {
	Title    string `json:"title"`
	ShowTime bool   `json:"showTime"`

	// 背景
	Wallpaper Wallpaper `json:"wallpaper"`

	// 布局
	LayoutMode string `json:"layoutMode"` // paged-horizontal/paged-vertical/paged-free/scroll

	// 网络入口偏好
	NetworkPrefer string `json:"networkPrefer"` // wanV4 / wanV6 / lan

	// 搜索
	DefaultSearchEngineID string         `json:"defaultSearchEngineId"`
	SearchEngines         []SearchEngine `json:"searchEngines"`
	SearchHistory         []string       `json:"searchHistory"` // 最多 24 条

	// 应用 & 分类
	Categories []Category `json:"categories"`
	Apps       []App      `json:"apps"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// 背景
type Wallpaper struct {
	Mode       string `json:"mode"`       // default / bing / custom
	Resolution string `json:"resolution"` // 1080 / uhd，只对 bing 有意义
	Blur       int    `json:"blur"`       // 0-12
	// Custom 自己上传的壁纸，形如 data/wallpaper/xxx.jpg。
	// 和 Mode 分开存：切到 bing 再切回来不用重新上传。
	Custom string `json:"custom"`
	// CSS 默认背景的样式，前端 .default-wallpaper 那条规则的大括号内容。
	// 空 = 用内置的那套渐变。只有登录后才能改，前端拿它塞进一个 <style>。
	CSS string `json:"css"`
}

// 搜索
type SearchEngine struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	ShortName string `json:"shortName"`
	URL       string `json:"url"`
	Color     string `json:"color"`
	Icon      string `json:"icon"` // 可选,/icons/xxx 或 data/icon/xxx
	Order     int    `json:"order"`
}

type Category struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Order int    `json:"order"`

	// RequireAuth 这个分组要登录后才看得见。
	//
	// 导航主页是公开页面，谁打开都能看——于是内网服务名、端口、内网 IP
	// 全都摆在明面上。把敏感的分组勾上这个，未登录访客拿到的配置里就不含
	// 这个分组和它下面的应用（后端过滤，不是前端藏一下，访客翻源码也翻不到）。
	//
	// 勾了之后访客那侧会看到一个"登录查看"的占位分组，知道这儿有东西，
	// 但看不到是什么。
	RequireAuth bool `json:"requireAuth"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type App struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Icon       string `json:"icon"`       // data/icon/xxx
	Color      string `json:"color"`      // tailwind gradient,无图标时背景用
	CategoryID string `json:"categoryId"` // 空 = 未分类

	PagedOrder  int `json:"pagedOrder"`  // 翻页模式全局序
	ScrollOrder int `json:"scrollOrder"` // 整页模式分类内序

	Type string `json:"type"` // stun / static

	// type=stun:
	StunDeviceID  uint `json:"stunDeviceId,omitempty"`
	StunServiceID uint `json:"stunServiceId,omitempty"`

	// type=static:
	Addresses *AppAddresses `json:"addresses,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// AppAddresses 应用的访问入口
type AppAddresses struct {
	WANv4 string `json:"wanV4"` // 公网 IPv4 入口 / URL
	WANv6 string `json:"wanV6"` // 公网 IPv6 入口
	LAN   string `json:"lan"`   // 内网 IPv4 入口
}
