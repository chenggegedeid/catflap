package home_api

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/chenggegedeid/catflap/middleware"
	"github.com/chenggegedeid/catflap/modules/home"
	"github.com/chenggegedeid/catflap/utils/res"

	"github.com/gin-gonic/gin"
)

/**
 * "我的壁纸"：把用户上传过的壁纸列出来，能反复切换、能删。
 *
 * 之前上传是"传一张覆盖一张" —— 想换回上一张只能重新上传，
 * 而且完全不知道自己传过什么。Sun-Panel 那类是做成"壁纸库"的，
 * 这个思路对：上传过的都是资产，应该看得见、选得回来。
 */

type WallpaperListRequest struct{}

type WallpaperItem struct {
	Path string `json:"path"`
	// URL 直接给前端 <img src> 用（带 / 前缀）
	URL  string `json:"url"`
	Size int64  `json:"size"`
	// ModTime 用来自增排序：新传的排在前面
	ModTime string `json:"modTime"`
}

func (HomeApi) WallpaperListView(c *gin.Context) {
	items := listWallpapers()
	res.OkWithData(items, c)
}

type WallpaperDeleteRequest struct {
	Path string `json:"path"`
}

func (HomeApi) WallpaperDeleteView(c *gin.Context) {
	cr := middleware.GetBindRequest[WallpaperDeleteRequest](c)

	/**
	 * 删除是这里唯一危险的操作，必须卡死三道：
	 *
	 * 1. 形状要对（validWallpaperPath：只能是 data/wallpaper/xxx.ext，扩展名在白名单）
	 * 2. **解析后必须仍在壁纸目录内** —— 防 `../../` 路径穿越。
	 *    只做字符串匹配不够：`data/wallpaper/../../etc/passwd` 形状也能糊弄过去
	 * 3. 必须真的存在
	 */
	if !validWallpaperPath(cr.Path) {
		res.FailWithMsg("壁纸路径不合法", c)
		return
	}
	abs, err := safeJoin(wallpaperDir, filepath.Base(cr.Path))
	if err != nil {
		res.FailWithMsg("壁纸路径不合法", c)
		return
	}
	if _, err := os.Stat(abs); err != nil {
		res.FailWithMsg("壁纸不存在", c)
		return
	}

	if err := os.Remove(abs); err != nil {
		res.FailWithMsg("删除失败:"+err.Error(), c)
		return
	}

	/**
	 * 删掉的正好是当前在用的 → 回退到内置壁纸库，不然会留一个
	 * 指向不存在文件的配置，下次读出来就是一块空白。
	 */
	home.Runtime.WithLock(func(cfg *home.Config) error {
		if cfg.Wallpaper.Mode == "custom" && cfg.Wallpaper.Custom == cr.Path {
			cfg.Wallpaper.Mode = home.VisibilityPublic // 占位，下面覆盖
			cfg.Wallpaper.Mode = "preset"
			cfg.Wallpaper.Custom = ""
			cfg.Wallpaper.CSS = defaultPresetCSS()
		}
		return nil
	})

	res.OkWithMsg("已删除", c)
}

func listWallpapers() []WallpaperItem {
	entries, err := os.ReadDir(wallpaperDir)
	if err != nil {
		return []WallpaperItem{}
	}

	out := make([]WallpaperItem, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !validWallpaperPath(filepath.ToSlash(filepath.Join(wallpaperDir, name))) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		rel := filepath.ToSlash(filepath.Join(wallpaperDir, name))
		out = append(out, WallpaperItem{
			Path:    rel,
			URL:     "/" + rel,
			Size:    info.Size(),
			ModTime: info.ModTime().Format(time.RFC3339),
		})
	}

	// 新传的排前面 —— 用户刚上传完想立刻看到它
	sort.Slice(out, func(i, j int) bool {
		return out[i].ModTime > out[j].ModTime
	})
	return out
}

/**
 * defaultPresetCSS 删除当前壁纸后回退用的内置主题。
 *
 * 和前端 PRESET_WALLPAPERS 的第一套（极光）保持一致 —— 两边各写一份字符串
 * 不是好做法，但前端是 TS、后端是 Go，真要共用得把整套 CSS 搬到 Go 里，
 * 不值得。这里加一条测试守住"两边一致"更实际（见 wallpaper_library_test.go）。
 */
func defaultPresetCSS() string {
	return strings.TrimSpace(`
radial-gradient(ellipse at 20% 20%, rgba(56,189,248,0.25), transparent 55%),
radial-gradient(ellipse at 80% 70%, rgba(168,85,247,0.22), transparent 55%),
linear-gradient(160deg, #0f2027, #203a43 55%, #2c5364)`)
}

/**
 * safeJoin 把"目录 + 文件名"拼成绝对路径，并保证结果仍在目录内。
 *
 * 为什么不直接 filepath.Join：Join 会保留 `..`，而删除是危险操作 ——
 * 传个 `../../config/xxx.json` 就能删到配置去。
 *
 * 做法：只用文件名（调用方已传 Base），再把解析后的绝对路径和目录的
 * 绝对路径比对前缀。**必须两端都解析成绝对路径再比**，否则
 * `data/wallpaper` 和 `./data/wallpaper` 会被判成不同前缀而误放行。
 */
func safeJoin(dir string, name string) (string, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	abs := filepath.Clean(filepath.Join(absDir, name))
	// 前缀必须是目录本身 + 分隔符，否则 data/wallpaper-2 也会被当成在目录内
	if abs != absDir && !strings.HasPrefix(abs, absDir+string(filepath.Separator)) {
		return "", os.ErrInvalid
	}
	return abs, nil
}
