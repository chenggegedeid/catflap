package proxy

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/chenggegedeid/catflap/modules/proxy/model"
	"github.com/sirupsen/logrus"
)

// errorPageCache 错误页配置的读取缓存。
//
// 为什么要有缓存而不是每次读 Runtime.Snapshot()：**不能在包级初始化里引用 Runtime**。
// 链路是 Runtime = NewRouter(...) → newReverseProxy → writeOriginUnreachable，
// 而 writeOriginUnreachable 要读配置 —— 直接写 Runtime.Snapshot() 就成了一个
// 包级变量的初始化环（Go 直接拒绝编译：initialization cycle for Runtime）。
//
// 用一个不带初始化依赖的原子变量绕开：写入方是 Runtime.Update 与 InitProxy
// （都是方法/函数，运行时才跑），读取方是错误页，两边不再有初始化顺序关系。
var errorPageCache atomic.Value

// currentErrorPage 当前错误页配置
func currentErrorPage() model.ErrorPage {
	v := errorPageCache.Load()
	if v == nil {
		return model.ErrorPage{}
	}
	return v.(model.ErrorPage)
}

// setErrorPageCache 配置变动时同步缓存（见 errorPageCache 的说明）
func setErrorPageCache(p model.ErrorPage) {
	errorPageCache.Store(p)
}

// 本文件的内容协商与缓存头处理借鉴 GoDoxy
// (github.com/yusing/godoxy, MIT License) 的 origin_unreachable_page.go，
// 页面文案与「指出是哪个后端连不上」为 Catflap 自有实现。

// setNoStoreHeaders 错误页绝对不能进缓存。
//
// 后端起来之后用户一刷新就该好，如果这个 502 被缓存了，
// 他会以为「修好了但还是坏的」，然后去重启整个 Catflap。
func setNoStoreHeaders(h http.Header) {
	h.Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	h.Set("Pragma", "no-cache")
	h.Set("Expires", "0")
}

// wantsHTML 只有「人用浏览器点进来」才值得给一整页 HTML。
// API 调用方收到一坨 HTML 只会让它的报错更难读。
func wantsHTML(req *http.Request) bool {
	if req == nil || req.Method != http.MethodGet {
		return false
	}
	accept := req.Header.Get("Accept")
	if accept == "" {
		// 没有 Accept 的裸 GET：只有访问根路径时才当成是人
		return req.URL != nil && req.URL.Path == "/"
	}
	return strings.Contains(accept, "text/html") || strings.Contains(accept, "*/*")
}

// writeOriginUnreachable 后端连不上时的 502。
//
// ⚠️ **不写后端地址**。以前这里会把 `site.Backend`（形如 192.168.9.6:5000）
// 明文打给外网访客 —— 那是内网拓扑，比域名清单更敏感：
// 谁都能看到你哪台机器在哪个端口跑着什么。后端地址去管理后台看，那里要登录。
func writeOriginUnreachable(w http.ResponseWriter, req *http.Request, host, backend string, err error) {
	setNoStoreHeaders(w.Header())

	// 错误原因只**进日志**，不给访客：
	// "connection refused" / "i/o timeout" 这类字眼会告诉对方后端在内网、
	// 是活的还是死的，配合别的信息就能判断拓扑。
	if err != nil {
		detail := describeBackendError(err)
		if detail != "" {
			logrus.Warnf("[proxy] 后端连不上（不对外显示）：%s → %s：%s", host, backend, detail)
		}
	}

	page := currentErrorPage()
	if page.UseCustom() {
		writeCustomPage(w, http.StatusBadGateway, page.CustomHTML, "502", "服务暂时不可用")
		return
	}

	if !wantsHTML(req) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprint(w, "502 后端连不上\n")
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusBadGateway)
	fmt.Fprint(w, originUnreachableTmpl)
}

// writeNoSite 收到了一个没有任何站点认领的 Host。
//
// ⚠️ **不列出已配置的域名**。以前这里会把全部域名打给任意来访者
// （handler 传进来的 hosts），等于任何人都能源源不断地枚举出这台机器上的站点。
// 域名拼没拼错是**自己**要知道的事，去管理后台看，或者看运行日志 ——
// 日志里每一次未匹配的 Host 都记着。
func writeNoSite(w http.ResponseWriter, req *http.Request, hostHeader string, hosts []string) {
	// 没匹配到站点也是"门禁拒绝"：扫描器常拿不存在的域名来试。
	// 这类响应不该计入访问统计（否则光扫域名就能把错误数刷上去）
	markDenied(w)
	setNoStoreHeaders(w.Header())
	// hosts 参数还在，只是不再往外显示 —— 留着是因为调用方传得顺手，
	// 而且将来要做"仅本站域名时给详细提示"时能用上。
	_ = hosts

	page := currentErrorPage()
	if page.UseCustom() {
		writeCustomPage(w, http.StatusNotFound, page.CustomHTML, "404", "没有找到请求的内容")
		return
	}

	if !wantsHTML(req) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		// 不回显 Host：那是来访者自己填的，回显没有意义，
		// 还会给"反射型"玩法留下空间（虽然这里已经转义过）。
		fmt.Fprint(w, "404 没有匹配的站点\n")
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	fmt.Fprint(w, noSiteTmpl)
}

// writeCustomPage 发用户自定义的错误页。
//
// 原样输出：这是管理员自己写的页面，转义了反而把他的 <style>/<script> 变成
// 可见文本。占位符替换只做字面替换，不做 HTML 转义 —— 值都是内部常量。
func writeCustomPage(w http.ResponseWriter, status int, tmpl, statusText, message string) {
	body := strings.NewReplacer(
		"{{status}}", statusText,
		"{{message}}", message,
	).Replace(tmpl)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

const pageStyle = `<style>
:root{color-scheme:light dark}
body{margin:0;min-height:100vh;display:flex;align-items:center;justify-content:center;
background:#0f1115;color:#e6e8ec;
font-family:system-ui,-apple-system,"Segoe UI","PingFang SC","Microsoft YaHei",sans-serif}
.card{max-width:34rem;padding:2.5rem 2rem;text-align:left}
h1{margin:0 0 .25rem;font-size:1.5rem;font-weight:600}
.code{color:#7d8590;font-size:.8rem;letter-spacing:.08em;margin:0 0 1.5rem}
p{margin:.5rem 0;line-height:1.7;color:#adb4bf}
code{background:#1c2029;padding:.15rem .4rem;border-radius:4px;
font-family:ui-monospace,SFMono-Regular,Consolas,monospace;font-size:.875rem;color:#e6e8ec}
.err{color:#e5786d}
.hint{font-size:.875rem}
ul{margin:.5rem 0;padding-left:1.25rem;line-height:1.9}
.foot{margin-top:1.75rem;font-size:.75rem;color:#5c636e}
</style>`

// 默认（脱敏）的两个页面刻意做成"什么也不说"：
// 不列已配置域名、不说后端地址、不回显来访者的 Host、不出现产品名。
// 页面样式是中性的深色卡片，看不出是谁家的反代。
const originUnreachableTmpl = `<!DOCTYPE html><html lang="zh-CN"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1"><title>502 服务暂时不可用</title>
` + pageStyle + `</head><body><div class="card">
<p class="code">502 SERVICE UNAVAILABLE</p>
<h1>服务暂时不可用</h1>
<p>请求已收到，但对应的服务这会儿没有响应。</p>
<p class="hint">通常是那台机器没开机、服务没启动，或者正在重启。稍后直接刷新本页即可。</p>
</div></body></html>`

const noSiteTmpl = `<!DOCTYPE html><html lang="zh-CN"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1"><title>404 没有找到</title>
` + pageStyle + `</head><body><div class="card">
<p class="code">404 NOT FOUND</p>
<h1>没有找到</h1>
<p>这个地址上没有可以提供的内容。</p>
<p class="hint">如果这是你自己的站点，请确认域名拼写、站点是否已启用，以及 DNS 是否解析到了这台机器。</p>
</div></body></html>`
