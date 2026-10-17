package backup

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/chenggegedeid/catflap/modules/audit"
	"github.com/chenggegedeid/catflap/modules/auth"
	"github.com/chenggegedeid/catflap/modules/cert"
	"github.com/chenggegedeid/catflap/modules/ddns"
	"github.com/chenggegedeid/catflap/modules/device"
	"github.com/chenggegedeid/catflap/modules/notify"
	"github.com/chenggegedeid/catflap/modules/proxy"
	"github.com/chenggegedeid/catflap/modules/stun"
	"github.com/chenggegedeid/catflap/modules/task"
	"github.com/chenggegedeid/catflap/modules/tunnel"
	"github.com/chenggegedeid/catflap/modules/update"
	"github.com/chenggegedeid/catflap/modules/webhook"
	"github.com/chenggegedeid/catflap/modules/wol"
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
/**
 * 模块注册表：备份要处理哪些配置。
 *
 * **路径一律引用各模块自己的 ConfigPath 常量，不在这里重写字符串。**
 *
 * 这不是洁癖：今天我第一版手写了路径，漏掉 6 个模块（devices/task/update/
 * webhook/wol…），于是"备份"其实不完整而我看不出来。
 * 引用常量之后，路径的真相只有一份（各模块自己那份），
 * 就算这里漏了一个模块，也不会出现"路径写错导致备份到空文件"这种更隐蔽的错。
 *
 * 只列**配置文件**，不列运行期数据（data/access、data/traffic 那些日志类
 * 不该进备份，它们随时能重建，只会让包变大）。
 */
var MODULES = []struct {
	Name  string
	Label string
	File  string
}{
	{"auth", "账号与登录", auth.ConfigPath},
	{"proxy", "反向代理", proxy.ConfigPath},
	{"stun", "穿透", stun.ConfigPath},
	{"ddns", "动态域名", ddns.ConfigPath},
	{"tunnel", "隧道", tunnel.ConfigPath},
	{"legacyProxy", "旧版反代配置", tunnel.LegacyProxyConfigPath},
	{"device", "设备", device.ConfigPath},
	{"audit", "审计", audit.ConfigPath},
	{"notify", "通知", notify.ConfigPath},
	{"cert", "证书", cert.ConfigPath},
	{"task", "定时任务", task.ConfigPath},
	{"update", "更新设置", update.ConfigPath},
	{"webhook", "Webhook", webhook.ConfigPath},
	{"wol", "网络唤醒", wol.ConfigPath},
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
