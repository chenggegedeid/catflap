package home

import (
	"os"
	"sort"
	"time"

	"github.com/chenggegedeid/catflap/utils/utilsFile"

	"github.com/sirupsen/logrus"
)

const ConfigPath = "config/homeConfig.json"

// LayoutSchemaV2 当前配置结构版本：流式网格（order / size）
const LayoutSchemaV2 = 2

// migrateLayout 把老布局（pagedOrder / scrollOrder）迁成流式网格（order / size）。
//
// 返回 true 表示改了东西，调用方要落盘 —— 否则迁移只在内存里生效，
// 下次启动又迁一遍，用户每次看到的顺序还可能不一样。
//
// 老的两个序：
//   - pagedOrder  翻页模式，page*pageSize+slot+1，1 开始
//   - scrollOrder 整页模式，分类内序
//
// 迁移规则：**只保相对顺序**。老配置里这两个数字的含义完全不同（一个是格子、
// 一个是序号），硬换算成新坐标没有意义。新布局是流式的，只要谁在前谁在后对，
// 排出来就和以前看起来一致。
func migrateLayout(cfg *Config) bool {
	if cfg.SchemaVersion >= LayoutSchemaV2 {
		// 已经迁过。但保险起见：用户手工改过配置、或者新增的应用没带 order 时，
		// 顺序可能是 0 —— 全 0 的话前端排出来会互相抢位置，补上。
		return fillMissingOrder(cfg)
	}

	// 按老顺序排序：两个序都有的话，翻页那个更能代表用户看到的顺序
	// （默认布局模式就是 paged-free）
	idx := make(map[string]int, len(cfg.Apps))
	for i, a := range cfg.Apps {
		idx[a.ID] = i
	}
	sort.SliceStable(cfg.Apps, func(i, j int) bool {
		return legacyRank(cfg.Apps[i]) < legacyRank(cfg.Apps[j])
	})
	_ = idx

	for i := range cfg.Apps {
		cfg.Apps[i].Order = i + 1
		if cfg.Apps[i].Size == "" {
			cfg.Apps[i].Size = defaultCardSize
		}
	}

	// 老布局模式不再有含义，清掉
	cfg.LegacyLayoutMode = ""
	cfg.SchemaVersion = LayoutSchemaV2
	cfg.UpdatedAt = time.Now()

	logrus.Infof("[home] 导航页布局已迁移到流式网格（%d 个应用）", len(cfg.Apps))
	return true
}

// legacyRank 老配置里这个应用的排序依据。两个序都取 0 时按创建时间兜底，
// 保证顺序是确定的（不然每次启动排序结果可能不同）
func legacyRank(a App) int64 {
	if a.LegacyPagedOrder > 0 {
		return int64(a.LegacyPagedOrder)
	}
	if a.LegacyScrollOrder > 0 {
		return int64(a.LegacyScrollOrder)
	}
	return 0
}

// fillMissingOrder 给没有 order（或 order 重复）的应用补上顺序
func fillMissingOrder(cfg *Config) bool {
	seen := make(map[int]bool, len(cfg.Apps))
	need := false
	for i := range cfg.Apps {
		if cfg.Apps[i].Order <= 0 || seen[cfg.Apps[i].Order] {
			need = true
			break
		}
		seen[cfg.Apps[i].Order] = true
		if cfg.Apps[i].Size == "" {
			need = true
			break
		}
	}
	if !need {
		return false
	}
	for i := range cfg.Apps {
		cfg.Apps[i].Order = i + 1
		if cfg.Apps[i].Size == "" {
			cfg.Apps[i].Size = defaultCardSize
		}
	}
	cfg.UpdatedAt = time.Now()
	return true
}

// defaultCardSize 默认卡片尺寸
const defaultCardSize = "1x1"

// ReadConfig 读取 home 配置，文件不存在则用默认值创建
func ReadConfig() (Config, error) {
	if fi, err := os.Stat(ConfigPath); os.IsNotExist(err) || (fi != nil && fi.Size() == 0) {
		return createConfig()
	}
	cfg, err := utilsFile.ReadJsonFile[Config](ConfigPath)
	if err != nil {
		logrus.Error("Home Config 读取失败：", err)
		return cfg, err
	}
	// 老版本配置没有 schemaVersion（= 0），在这里就地升级。
	// 落盘的动作交给 init（ReadConfig 自己不写文件，保持"读就是读"）
	return cfg, nil
}

// createConfig 首次启动时写入默认值
func createConfig() (Config, error) {
	now := time.Now()
	cfg := Config{
		Title:    "Catflap",
		ShowTime: true,
		Wallpaper: Wallpaper{
			Mode:       "bing",
			Resolution: "1080",
			Blur:       0,
		},
		NetworkPrefer: "wanV4",

		SchemaVersion: LayoutSchemaV2,

		DefaultSearchEngineID: "bing",
		SearchEngines:         defaultSearchEngines(),
		SearchHistory:         []string{},

		Categories: defaultCategories(now),
		Apps:       []App{},

		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := utilsFile.EnsurePrivateDir("config"); err != nil {
		logrus.Error("创建 config 目录失败：", err)
		return cfg, err
	}
	if err := utilsFile.WriteJsonFile(ConfigPath, cfg); err != nil {
		logrus.Error("Home Config 写入失败：", err)
		return cfg, err
	}
	return cfg, nil
}

// UpdateConfig 写回当前 Runtime.Config（调用者已自行加锁/修改 UpdatedAt 时除外）
func UpdateConfig(cfg Config) error {
	cfg.UpdatedAt = time.Now()
	if err := utilsFile.WriteJsonFile(ConfigPath, cfg); err != nil {
		logrus.Error("Home Config 写入失败：", err)
		return err
	}
	return nil
}

func defaultSearchEngines() []SearchEngine {
	return []SearchEngine{
		{ID: "bing", Name: "必应", ShortName: "B", URL: "https://www.bing.com/search?q=", Color: "from-sky-500 to-blue-600", Icon: "/icons/bing.com.ico", Order: 1},
		{ID: "google", Name: "谷歌", ShortName: "G", URL: "https://www.google.com/search?q=", Color: "from-red-500 to-yellow-500", Icon: "/icons/google.com.svg", Order: 2},
		{ID: "duckduckgo", Name: "DuckDuckGo", ShortName: "D", URL: "https://duckduckgo.com/?q=", Color: "from-orange-500 to-red-500", Icon: "/icons/dackdackgo.com.svg", Order: 3},
		{ID: "brave", Name: "Brave", ShortName: "Br", URL: "https://search.brave.com/search?q=", Color: "from-orange-600 to-red-700", Icon: "/icons/brave.com.svg", Order: 4},
		{ID: "github", Name: "GitHub", ShortName: "Gh", URL: "https://github.com/search?q=", Color: "from-gray-700 to-gray-900", Icon: "/icons/github.com.svg", Order: 5},
		{ID: "zhihu", Name: "知乎", ShortName: "Zh", URL: "https://www.zhihu.com/search?type=content&q=", Color: "from-blue-500 to-indigo-600", Icon: "/icons/zhihu.com.ico", Order: 6},
		{ID: "bilibili", Name: "B站", ShortName: "B", URL: "https://search.bilibili.com/all?keyword=", Color: "from-pink-400 to-pink-600", Icon: "/icons/bilibili.com.ico", Order: 7},
	}
}

func defaultCategories(now time.Time) []Category {
	return []Category{
		{ID: "productivity", Name: "生产力", Order: 1, CreatedAt: now, UpdatedAt: now},
		{ID: "tools", Name: "工具", Order: 2, CreatedAt: now, UpdatedAt: now},
		{ID: "entertainment", Name: "娱乐", Order: 3, CreatedAt: now, UpdatedAt: now},
	}
}
