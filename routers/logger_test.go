package routers

import "testing"

// 桌面免登录 secret 会随首屏 URL 打进来，绝不能出现在日志里
func TestSanitizePath(t *testing.T) {
	cases := []struct{ in, want string }{
		{"/catflap/", "/catflap/"},
		{"/api/version", "/api/version"},
		{"/catflap/?desktop_secret=deadbeef", "/catflap/"},
		{"/catflap/?desktop_secret=deadbeef&x=1", "/catflap/?x=1"},
		{"/catflap/?x=1&desktop_secret=deadbeef", "/catflap/?x=1"},
		{"/api/auth/status?desktop_secret=deadbeef", "/api/auth/status"},
		// 不含 secret 的查询串原样保留，顺序也不动
		{"/catflap/?other=1", "/catflap/?other=1"},
	}
	for _, c := range cases {
		if got := sanitizePath(c.in); got != c.want {
			t.Errorf("sanitizePath(%q) = %q，期望 %q", c.in, got, c.want)
		}
	}
}

// 参数值里出现同名字样（比如别的参数的值刚好是 desktop_secret=xx）时，
// 也要保证真正的 desktop_secret 被删掉
func TestSanitizePathRemovesAllSecretParams(t *testing.T) {
	got := sanitizePath("/catflap/?desktop_secret=a&desktop_secret=b")
	if got != "/catflap/" {
		t.Fatalf("重复的 desktop_secret 应全部删除，得到 %q", got)
	}
}
