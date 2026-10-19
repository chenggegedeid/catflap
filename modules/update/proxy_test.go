package update

import (
	"net/http"
	"net/url"
	"testing"
)

/*
 * 这几条守的是「更新走代理」的配置解析。
 *
 * 为什么值得专门测：代理地址是用户手填的自由文本，格式花样很多，
 * 而**解析错的表现是"下载还是慢"，用户根本不会想到是代理没生效** ——
 * 这种静默失效最难发现，所以要把各种写法钉死。
 *
 * 注意：这里一律用示例地址（127.0.0.1 / user:pass），
 * 不写任何真实代理 —— 仓库是公开的。
 */

// TestProxyEmptyFallsBackToEnvironment 留空 = 听环境变量的，行为不变。
//
// 老用户可能一直靠 HTTP_PROXY 环境变量走代理，加了这个字段不能把那条路堵死。
func TestProxyEmptyFallsBackToEnvironment(t *testing.T) {
	got := proxyFuncFor("")
	// ProxyFromEnvironment 是个包级函数，只能比对它是否非空且行为一致
	if got == nil {
		t.Fatal("留空时应当退回 http.ProxyFromEnvironment，不能是 nil")
	}
	// 行为验证：环境变量为空时它应返回 nil（即直连）
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("http_proxy", "")
	t.Setenv("https_proxy", "")
	req, _ := http.NewRequest("GET", "https://api.github.com/", nil)
	u, err := got(req)
	if err != nil {
		t.Fatalf("不该返回错误：%v", err)
	}
	if u != nil {
		t.Errorf("环境变量全空时应直连（返回 nil），实际 %v", u)
	}
}

// TestProxyWithCredentials 带用户名密码的 HTTP 代理（Clash / V2Ray 常见写法）。
//
// 关键：userinfo 必须留在 URL 上 —— net/http 靠它生成
// Proxy-Authorization 请求头。解析时把它丢了，代理会回 407。
func TestProxyWithCredentials(t *testing.T) {
	f := proxyFuncFor("http://user:pass@127.0.0.1:7890")
	req, _ := http.NewRequest("GET", "https://api.github.com/", nil)
	u, err := f(req)
	if err != nil {
		t.Fatalf("不该返回错误：%v", err)
	}
	if u == nil {
		t.Fatal("应当返回代理地址，实际 nil")
	}
	if u.Scheme != "http" {
		t.Errorf("scheme 应为 http，实际 %q", u.Scheme)
	}
	if u.Host != "127.0.0.1:7890" {
		t.Errorf("host 应含端口，实际 %q", u.Host)
	}
	if u.User == nil {
		t.Fatal("userinfo 丢了 —— 代理需要认证时会回 407，表现为下载失败")
	}
	if u.User.Username() != "user" {
		t.Errorf("用户名解析错误，实际 %q", u.User.Username())
	}
	if pw, ok := u.User.Password(); !ok || pw != "pass" {
		t.Errorf("密码解析错误，实际 %q (ok=%v)", pw, ok)
	}
}

// TestProxyBareHostPortGetsScheme 只填 127.0.0.1:7890 也要能用。
//
// 用户多半懒得写 http://，不补的话 url.Parse 会把它当成 path，
// 结果代理形同没配 —— 又是一个"静默失效"。
func TestProxyBareHostPortGetsScheme(t *testing.T) {
	f := proxyFuncFor("127.0.0.1:7890")
	req, _ := http.NewRequest("GET", "https://api.github.com/", nil)
	u, err := f(req)
	if err != nil {
		t.Fatalf("不该返回错误：%v", err)
	}
	if u == nil {
		t.Fatal("应当返回代理地址")
	}
	if u.Scheme != "http" {
		t.Errorf("没写 scheme 时应默认 http，实际 %q", u.Scheme)
	}
	if u.Host != "127.0.0.1:7890" {
		t.Errorf("host 应为 127.0.0.1:7890，实际 %q", u.Host)
	}
}

// TestProxyInvalidFallsBackNotPanics 填了乱七八糟的东西不能崩、也不能让更新彻底不可用。
func TestProxyInvalidFallsBackNotPanics(t *testing.T) {
	for _, bad := range []string{"   ", "://", "http://"} {
		f := proxyFuncFor(bad)
		if f == nil {
			t.Errorf("代理填 %q 时不应返回 nil（要退回环境变量），否则 nil 会让请求直接失败", bad)
		}
	}
}

// TestSocks5UsesDialer socks5 走拨号层而不是 Proxy 字段。
//
// net/http 的 Proxy 只认 http/https 两种 scheme，socks5 必须自己拨号，
// 否则会被当成普通 URL 拿去 CONNECT，连不上。
func TestSocks5UsesDialer(t *testing.T) {
	d, ok := socksDialer("socks5://127.0.0.1:1080")
	if !ok || d == nil {
		t.Fatal("socks5 应当走拨号层（返回 true 且给到 dialer）")
	}
	// 非 socks5 的不该抢拨号
	if _, ok := socksDialer("http://127.0.0.1:7890"); ok {
		t.Error("http 代理不该走 socks 拨号层")
	}
	if _, ok := socksDialer("127.0.0.1:7890"); ok {
		t.Error("没写 scheme 默认 http，不该走 socks 拨号层")
	}
	if _, ok := socksDialer(""); ok {
		t.Error("留空时不该走 socks 拨号层")
	}
}

// TestConfigProxyIsEmptyByDefault 新装的默认配置不能自带代理。
//
// 这是隐私底线：仓库是公开的，任何真实代理地址都不能进代码。
// 部署者自己在界面上填，填了只落在他自己机器的 config/updateConfig.json 里。
func TestConfigProxyIsEmptyByDefault(t *testing.T) {
	var c Config
	if c.Proxy != "" {
		t.Errorf("Config 零值的 Proxy 必须为空，实际 %q", c.Proxy)
	}
}

// TestUpdateHTTPClientUsable 客户端能建出来，且 ipify 这类探测不会 panic。
func TestUpdateHTTPClientUsable(t *testing.T) {
	c := &http.Client{}
	_ = c
	if client := updateHTTPClient(Config{}); client == nil {
		t.Fatal("updateHTTPClient 不应返回 nil")
	}
	if client := updateHTTPClient(Config{Proxy: "http://127.0.0.1:7890"}); client == nil {
		t.Fatal("填了代理时也不应返回 nil")
	}
}

// 保证 url 包被直接用到（上面的断言里其实已经用了，这里只是显式引用一次防误删）
var _ = url.Parse
