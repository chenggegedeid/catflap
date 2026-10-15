package proxy

import "testing"

// 后端地址指向 Catflap 自己的后台端口必须被拦住。
//
// 这不是理论风险：后端填 127.0.0.1:<管理端口> 会把管理后台转发到公网，
// 任何人都能在上面爆破密码（/api/auth/login 是公开的），猜中就完全接管。
// 实测过整条链路能跑通，所以这条校验必须有，且不能被改坏。
func TestBackendHitsAdminPort(t *testing.T) {
	const admin = 3791

	blocked := []string{
		"127.0.0.1:3791",
		"localhost:3791",
		"[::1]:3791",
		" 127.0.0.1:3791 ", // 前后空格也不能放过
	}
	for _, b := range blocked {
		if !backendHitsAdminPort(b, admin) {
			t.Errorf("%q 应该被拦（指向本机 + 管理端口）", b)
		}
	}

	// 正常配置不能误伤：服务确实可能跑在本机，只是端口不同
	allowed := []string{
		"127.0.0.1:8080",
		"127.0.0.1:5000",
		"192.168.1.20:8080", // 别的机器
		"example.com:3791",  // 别的机器恰好也开 3791，不该拦
	}
	for _, a := range allowed {
		if backendHitsAdminPort(a, admin) {
			t.Errorf("%q 不该被拦（不是本机，或端口不是管理端口）", a)
		}
	}
}

// 畸形输入不能崩，也不能误判
func TestBackendHitsAdminPortMalformed(t *testing.T) {
	const admin = 3791
	for _, bad := range []string{"", "127.0.0.1", ":3791", "127.0.0.1:abc", "127.0.0.1:99999"} {
		if backendHitsAdminPort(bad, admin) {
			t.Errorf("%q 是畸形输入，不该被判成命中管理端口", bad)
		}
	}

	// 管理端口没取到（0）时不做判断
	if backendHitsAdminPort("127.0.0.1:3791", 0) {
		t.Error("管理端口为 0 时不应拦截")
	}
}
