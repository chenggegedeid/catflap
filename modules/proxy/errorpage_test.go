package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/chenggegedeid/catflap/modules/proxy/model"
)

// TestNoSitePageLeaksNothing 默认错误页不能泄露任何内部信息。
//
// 守的是一条真实的安全问题（用户实测发现）：以前 404 页面会把**已配置的域名
// 整列**打给任何带着任意 Host 头发来的请求 —— 不需要登录，一次请求就能把
// 这台机器上的站点枚举一遍。502 页面还会把内网后端地址一起送出去。
// 那等于替攻击者做了一遍子域名枚举 + 内网拓扑测绘。
//
// 所以这个测试不是"文案检查"：secrets 里每一条都是能被拿去用的东西。
func TestNoSitePageLeaksNothing(t *testing.T) {
	secrets := []string{
		"nas.example.com", // 已配置的域名
		"192.168.9.6",     // 内网后端 IP
		":5000",           // 内网后端端口
		"Catflap",         // 产品名（可用来批量搜索暴露实例）
		"反向代理",            // 同上，中文指纹
	}

	cases := []struct {
		name   string
		status int
		write  func(w http.ResponseWriter, r *http.Request)
	}{
		{
			name:   "404 没匹配到站点",
			status: http.StatusNotFound,
			write: func(w http.ResponseWriter, r *http.Request) {
				// 第二个参数就是「已配置的域名列表」——正是以前会被打出去的东西
				writeNoSite(w, r, "evil.example.net", []string{"nas.example.com"})
			},
		},
		{
			name:   "502 后端连不上",
			status: http.StatusBadGateway,
			write: func(w http.ResponseWriter, r *http.Request) {
				writeOriginUnreachable(w, r, "nas.example.com", "192.168.9.6:5000", errBoom{})
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// 默认（脱敏）模式
			setErrorPageCache(model.ErrorPage{})
			defer setErrorPageCache(model.ErrorPage{})

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Accept", "text/html")
			w := httptest.NewRecorder()

			tc.write(w, req)

			if w.Code != tc.status {
				t.Fatalf("状态码不对：期望 %d，实际 %d", tc.status, w.Code)
			}
			body := w.Body.String()
			for _, s := range secrets {
				if strings.Contains(body, s) {
					t.Errorf("默认错误页泄露了 %q：\n%s", s, body)
				}
			}
		})
	}
}

// TestCustomErrorPageIsServed 自定义模式下要真的把用户的页面发出去。
func TestCustomErrorPageIsServed(t *testing.T) {
	setErrorPageCache(model.ErrorPage{
		Mode:       "custom",
		CustomHTML: "<html><body><h1>{{status}}</h1><p>{{message}}</p></body></html>",
	})
	defer setErrorPageCache(model.ErrorPage{})

	for _, tc := range []struct {
		status int
		write  func(w http.ResponseWriter, r *http.Request)
		text   string
	}{
		{http.StatusNotFound, func(w http.ResponseWriter, r *http.Request) {
			writeNoSite(w, r, "whatever", nil)
		}, "404"},
		{http.StatusBadGateway, func(w http.ResponseWriter, r *http.Request) {
			writeOriginUnreachable(w, r, "nas.example.com", "192.168.9.6:5000", nil)
		}, "502"},
	} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Accept", "text/html")
		w := httptest.NewRecorder()
		tc.write(w, req)

		if w.Code != tc.status {
			t.Fatalf("状态码不对：期望 %d，实际 %d", tc.status, w.Code)
		}
		body := w.Body.String()
		if !strings.Contains(body, "<h1>"+tc.text+"</h1>") {
			t.Errorf("占位符没替换成 %s：\n%s", tc.text, body)
		}
		if strings.Contains(body, "{{") {
			t.Errorf("还有没替换掉的占位符：\n%s", body)
		}
	}
}

// errBoom 一个故意的后端错误，用来触发 502 分支
type errBoom struct{}

func (errBoom) Error() string { return "dial tcp 192.168.9.6:5000: connect: connection refused" }
