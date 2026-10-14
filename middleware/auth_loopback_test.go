package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestIsLoopbackAddr(t *testing.T) {
	cases := []struct {
		addr string
		want bool
	}{
		{"127.0.0.1:1234", true},
		{"127.0.0.2:80", true}, // 整个 127.0.0.0/8 都是回环
		{"[::1]:443", true},    // IPv6 回环带端口
		{"::1", true},          // IPv6 回环不带端口
		{"192.168.1.5:1234", false},
		{"10.0.0.1:80", false},
		{"[fe80::1]:80", false}, // 链路本地不等于回环
		{"8.8.8.8:53", false},
		{"", false},        // 空地址不放行
		{"garbage", false}, // 解析异常不放行
	}
	for _, tc := range cases {
		if got := isLoopbackAddr(tc.addr); got != tc.want {
			t.Errorf("isLoopbackAddr(%q) = %v, want %v", tc.addr, got, tc.want)
		}
	}
}

func newLoopbackRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/setup", LoopbackOnly, func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	return r
}

func doSetup(r *gin.Engine, remoteAddr string, headers map[string]string) int {
	req := httptest.NewRequest(http.MethodPost, "/setup", nil)
	req.RemoteAddr = remoteAddr
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code
}

func TestLoopbackOnlyAllowsLocal(t *testing.T) {
	r := newLoopbackRouter()
	if code := doSetup(r, "127.0.0.1:54321", nil); code != http.StatusOK {
		t.Fatalf("本机发起应放行，得到 %d", code)
	}
	if code := doSetup(r, "[::1]:54321", nil); code != http.StatusOK {
		t.Fatalf("IPv6 本机发起应放行，得到 %d", code)
	}
}

func TestLoopbackOnlyRejectsRemote(t *testing.T) {
	r := newLoopbackRouter()
	if code := doSetup(r, "192.168.1.50:54321", nil); code != http.StatusForbidden {
		t.Fatalf("跨机发起应被拒，得到 %d", code)
	}
	if code := doSetup(r, "203.0.113.7:443", nil); code != http.StatusForbidden {
		t.Fatalf("公网发起应被拒，得到 %d", code)
	}
}

// 关键回归：伪造代理头不能绕过。gin 默认信任全部代理时，
// c.ClientIP() 会读 X-Forwarded-For，从而被冒充成 127.0.0.1。
// 本实现取 Request.RemoteAddr，故以下伪造头必须无效。
func TestLoopbackOnlyIgnoresForgedProxyHeaders(t *testing.T) {
	r := newLoopbackRouter()
	forged := map[string]string{
		"X-Forwarded-For": "127.0.0.1",
		"X-Real-IP":       "127.0.0.1",
		"Forwarded":       "for=127.0.0.1",
	}
	if code := doSetup(r, "192.168.1.50:54321", forged); code != http.StatusForbidden {
		t.Fatalf("伪造 X-Forwarded-For 后仍应被拒，得到 %d", code)
	}
}
