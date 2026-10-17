package backup

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"
)

/**
 * 模块注册表：备份要处理哪些配置。
 *
 * 集中在一处，是为了"加新模块时只改这里" —— 否则每个模块散在各处，
 * 加一个就漏一个备份入口（这类漏同步我们今天已经踩过三次了）。
 *
 * 只列**配置文件**，不列运行期数据（data/access、data/traffic 那些日志类
 * 不该进备份，它们随时能重建，只会让包变大）。
 */
var MODULES = []struct {
	Name  string
	Label string
	File  string
}{
	{"home", "导航页", "config/homeConfig.json"},
	{"auth", "账号与登录", "config/authConfig.json"},
	{"proxy", "反向代理", "config/proxyConfig.json"},
	{"stun", "穿透", "config/stunConfig.json"},
	{"ddns", "动态域名", "config/ddnsConfig.json"},
	{"notify", "通知", "config/notify.json"},
	{"cert", "证书", "config/certConfig.json"},
}

// 上传资产目录（图标、壁纸）。日志/统计类不在此列。
var ASSETS = []struct {
	Kind string
	Dir  string
}{
	{"icon", "data/icon"},
	{"wallpaper", "data/wallpaper"},
}

/**
 * safeJoin 拼接路径并保证结果仍在 root 内。
 *
 * 两个方向都要用：
 *   导出（写 zip）—— 保证不会把 data/wallpaper 以外的东西打进去
 *   导入（解 zip）—— 保证不会解压到 config/ 以外（**zip slip**：
 *     备份包里放一个 `../../etc/cron.d/xxx` 就能往系统里写文件）
 *
 * 两端都 Abs 再比前缀：否则 ./config 和 config 会被判成不同前缀而误放行。
 * 前缀必须是"目录 + 分隔符"：否则 config-2 也会被当成在 config 内。
 */
func safeJoin(root string, name string) (string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	abs := filepath.Clean(filepath.Join(absRoot, name))
	if abs != absRoot && !strings.HasPrefix(abs, absRoot+string(filepath.Separator)) {
		return "", os.ErrInvalid
	}
	return abs, nil
}

/**
 * writeFileToZip 把一个磁盘文件写进 zip，包内路径用相对路径。
 *
 * 包内路径统一用**正斜杠**（zip 规范），且不带盘符/前导分隔符 ——
 * 否则某些解压工具会解出奇怪的目录结构。
 */
func writeFileToZip(w *zip.Writer, src string, inZip string) (int64, error) {
	f, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return 0, err
	}
	if info.IsDir() {
		return 0, nil
	}

	hdr, err := zip.FileInfoHeader(info)
	if err != nil {
		return 0, err
	}
	hdr.Name = filepath.ToSlash(inZip)
	hdr.Method = zip.Deflate

	dst, err := w.CreateHeader(hdr)
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(dst, f)
	if err != nil {
		return 0, err
	}
	return n, nil
}
