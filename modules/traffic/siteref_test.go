package traffic

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// 站点 ID 要能从"外层包装"传到"内层 handler"再读回来。
//
// 这是踩过的坑：一开始用 context 传，内层 `r = r.WithContext(...)` 只改
// 它那个局部变量，外层读到的 r 还是旧的 —— 表现是所有流量都记到
// "未匹配站点"（ID 0）名下，而且**不报任何错**，只是统计里分不出服务。
func TestSiteRefCrossesHandlerBoundary(t *testing.T) {
	var got uint

	// 模拟真实结构：外层创建容器，内层（下一个 handler）填，外层读
	outer := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r = NewSiteRef(r)
			next.ServeHTTP(w, r)
			got = SiteIDFromRequest(r)
		})
	}

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 内层匹配到站点 42
		SetSiteID(r, 42)
	})

	req := httptest.NewRequest("GET", "http://nas.example.com/", nil)
	outer(inner).ServeHTTP(httptest.NewRecorder(), req)

	if got != 42 {
		t.Fatalf("站点 ID 没传到外层：期望 42，实际 %d（说明又用回 context 了）", got)
	}
}

// 没挂容器时读出来是 0，不能崩
func TestSiteIDWithoutRef(t *testing.T) {
	req := httptest.NewRequest("GET", "http://x/", nil)
	if got := SiteIDFromRequest(req); got != 0 {
		t.Fatalf("没挂容器时应为 0，实际 %d", got)
	}
	// 没挂容器时 SetSiteID 也不能崩
	SetSiteID(req, 7)
}

// 容器是按请求独立的：两个请求的站点 ID 不能串
func TestSiteRefIsPerRequest(t *testing.T) {
	r1 := NewSiteRef(httptest.NewRequest("GET", "http://a/", nil))
	r2 := NewSiteRef(httptest.NewRequest("GET", "http://b/", nil))

	SetSiteID(r1, 1)
	SetSiteID(r2, 2)

	if SiteIDFromRequest(r1) != 1 || SiteIDFromRequest(r2) != 2 {
		t.Fatal("两个请求的站点 ID 串了")
	}
}
