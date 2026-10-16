package proxy

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/chenggegedeid/catflap/core"
	"github.com/chenggegedeid/catflap/modules/proxy/model"
)

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

// prepConfigDir 切到临时目录并建好 config/。
//
// SaveSite 会落盘到 config/proxyConfig.json（见 modules/stun 那边踩过的坑：
// 不切目录会在源码树里留下 modules/proxy/config/，已经误提交过两次）。
func prepConfigDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	if err := os.MkdirAll("config", 0o755); err != nil {
		t.Fatal(err)
	}
}

// 开关关着：后端指向后台端口必须被拦（这是默认行为，也是升级后的行为）
func TestSaveSiteBlocksAdminPortByDefault(t *testing.T) {
	prepConfigDir(t)
	// 默认配置（ExposeAdmin 零值 = false）
	_, err := SaveSite(model.Site{
		Hosts:   []string{"panel.example.com"},
		Backend: fmt.Sprintf("127.0.0.1:%d", core.ListenPort()),
	})
	if err == nil {
		t.Fatal("默认应当拦住把后台挂出去的站点")
	}
	if !strings.Contains(err.Error(), "允许通过反代暴露管理后台") {
		t.Errorf("报错里要告诉用户怎么合法地开，实际是：%v", err)
	}
}

// 开关打开：放行，但要真的能保存成功
func TestSaveSiteAllowsAdminPortWhenExposeEnabled(t *testing.T) {
	prepConfigDir(t)
	if err := Runtime.Update(func(cfg *model.ProxyConfig) error {
		cfg.ExposeAdmin = true
		return nil
	}); err != nil {
		t.Fatalf("打开开关失败：%v", err)
	}
	t.Cleanup(func() {
		_ = Runtime.Update(func(cfg *model.ProxyConfig) error {
			cfg.ExposeAdmin = false
			return nil
		})
	})

	out, err := SaveSite(model.Site{
		Hosts:   []string{"panel.example.com"},
		Backend: fmt.Sprintf("127.0.0.1:%d", core.ListenPort()),
	})
	if err != nil {
		t.Fatalf("开关打开后应当放行，实际报错：%v", err)
	}
	if out.Backend == "" {
		t.Error("保存结果里后端地址不该为空")
	}
}
