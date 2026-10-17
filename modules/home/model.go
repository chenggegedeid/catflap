package home

import "time"

const SearchHistoryMax = 24

type Config struct {
	Title    string `json:"title"`
	ShowTime bool   `json:"showTime"`

	// 背景
	Wallpaper Wallpaper `json:"wallpaper"`

	// 布局
	//
	// LayoutMode 旧版布局模式（左右翻页/上下翻页/自由翻页/整页滚动）。
	// 新架构只有一种：**流式网格**（列数随窗口宽度变，往下排），所以这个字段不再有含义。
	//
	// 保留 json 名是为了读老配置；迁移完会被清空，之后不再写进文件
	// （和 proxy 的 LegacyHTTPPort 一个套路）。
	LegacyLayoutMode string `json:"layoutMode,omitempty"`

	// SchemaVersion 配置结构版本。
	//   1 = 老布局（pagedOrder / scrollOrder）
	//   2 = 流式网格（order / size）
	//   3 = 三级可见性（visibility + 分组口令）
	// ReadConfig 靠它判断要不要跑迁移。
	//
	// **为什么要有版本号而不是"看字段有没有值"**：老配置里 pagedOrder 可能全是 0
	// （比如用户从来没拖过），光看值分不清"没迁移"和"迁移过但用户没排"。
	SchemaVersion int `json:"schemaVersion"`

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

	/**
	 * Visibility 这个分组谁能看。
	 *
	 *   public 任何人（未登录也行）
	 *   login  登录后
	 *   lock   即使登录了，也要再输一次**这个分组自己的口令**
	 *
	 * lock 解决的是什么：同一个浏览器上，家人打开就能看到你的私密分组。
	 * 登录态解决不了这个 —— 你登着，别人坐过来就能看。
	 */
	Visibility string `json:"visibility"`

	// PasswordHash lock 分组的口令哈希（pwd.HashPassword 产物）。
	// 只有 Visibility=lock 才有值；改成别的可见性时一并清掉，
	// 免得留一份没用的哈希在配置文件里。
	PasswordHash string `json:"passwordHash,omitempty"`

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

	// Order 排列顺序，从 1 开始，全局唯一。
	//
	// **存顺序而不是存网格坐标**：新布局是流式的，列数随窗口宽度变
	// （宽屏 8 列、手机 4 列）。存死 row/col 的话，换个宽度卡片就跑到网格外了。
	// 所以只存"谁在前谁在后"，具体落在第几行第几列由渲染时按当前列数算
	// （见 web/home/src/lib/grid.ts 的 normalize）。
	Order int `json:"order"`
	// Size 卡片尺寸：1x1 / 1x2 / 2x1 / 2x2 / 2x4（宽×高）。空 = 1x1。
	Size string `json:"size"`

	// 下面两个是旧版字段，只用来读老配置。
	// 迁移成上面的 Order/Size 后它们会一并清空，之后不再写进文件。
	//
	// TODO(M4)：新前端落地后把这两个字段和 home/layout 接口一起删掉。
	// 现在之所以还留着值（而不是立刻清零），是因为旧版导航页还在用它们渲染 ——
	// 直接清了会让公开页面在 M4 之前一直是坏的。
	LegacyPagedOrder  int `json:"pagedOrder,omitempty"`
	LegacyScrollOrder int `json:"scrollOrder,omitempty"`

	Type string `json:"type"` // stun / static

	// type=stun:
	StunDeviceID  uint `json:"stunDeviceId,omitempty"`
	StunServiceID uint `json:"stunServiceId,omitempty"`

	// type=static:
	Addresses *AppAddresses `json:"addresses,omitempty"`

	// ManualAddresses 用户手填的入口，用来补充「自动探测拿不到」的那几类。
	//
	// 最典型的场景：一台机器只有公网 IPv6（家宽没 IPv4），穿透给的是 v4 地址，
	// 而 v6 和「内网怎么进」只有用户自己知道。以前只能再复制一个图标分别填，
	// 现在填在这里，同一个图标就覆盖三种入口。
	//
	// 合并规则见 hydrate.go：**手填优先，自动探测兜底**。唯一的例外是
	// WANv4 —— 那是穿透的本职，始终以实时探测为准（隧道重打后地址会变）。
	ManualAddresses *AppAddresses `json:"manualAddresses,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// AppAddresses 应用的访问入口
type AppAddresses struct {
	WANv4 string `json:"wanV4"` // 公网 IPv4 入口 / URL
	WANv6 string `json:"wanV6"` // 公网 IPv6 入口
	LAN   string `json:"lan"`   // 内网 IPv4 入口
}

// DefaultCardSize 默认卡片尺寸。
//
// 新应用一律从 1×1 开始 —— 尺寸是用户**主动**想放大时才改的，
// 系统不该替他决定"这个服务看起来重要，给个 2×2"。
func DefaultCardSize() string { return defaultCardSize }

// ValidCardSize 合法的卡片尺寸（宽×高）
func ValidCardSize(s string) bool {
	switch s {
	case "1x1", "1x2", "2x1", "2x2", "2x4":
		return true
	}
	return false
}

// SetAppSize 设置应用尺寸。
//
// 尺寸不对要报错而不是默默改成 1×1 —— 用户在右键菜单里选了 2×2，
// 存成了 1×1，界面上看不出哪里错了，只会觉得"点了没反应"。
func SetAppSize(a *App, size string) error {
	if !ValidCardSize(size) {
		return ErrBadCardSize
	}
	a.Size = size
	return nil
}

// VisibilityPublic / VisibilityLogin / VisibilityLock 分组的三种可见性
const (
	VisibilityPublic = "public"
	VisibilityLogin  = "login"
	VisibilityLock   = "lock"
)

// NormalizeVisibility 把任意输入收敛成合法可见性。
//
// 不认识的（空串、老配置里没这个字段）一律按 public ——
// 这是**最保守**的方向吗？不是，但它是对的：老用户的分组本来就是公开的，
// 升级后突然把人家的分组藏起来，比什么都难受。真要藏，用户自己改。
func NormalizeVisibility(v string) string {
	switch v {
	case VisibilityPublic, VisibilityLogin, VisibilityLock:
		return v
	}
	return VisibilityPublic
}

// NeedsAuth 这个分组是否需要登录态
func (c Category) NeedsAuth() bool { return c.Visibility == VisibilityLogin }

// NeedsUnlock 这个分组是否需要**再验一次口令**（登录了也要验）
func (c Category) NeedsUnlock() bool { return c.Visibility == VisibilityLock }
