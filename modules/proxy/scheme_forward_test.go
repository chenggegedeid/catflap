package proxy

import (
	"testing"

	"github.com/chenggegedeid/catflap/modules/proxy/model"
)

// Scheme 三态的语义（这是这次改动的核心）：
//   - auto  = HTTP 一直有，勾了 HTTPS 再额外挂一份（升级前 HTTPS=true 的行为）
//   - http  = 只挂 HTTP
//   - https = 只挂 HTTPS
func TestSchemeBinding(t *testing.T) {
	site := func(scheme string, https bool) model.Site {
		return model.Site{ID: 1, Hosts: []string{"a.com"}, Enabled: true, Scheme: scheme, HTTPS: https}
	}

	cases := []struct {
		name       string
		scheme     string
		https      bool
		wantHTTP   bool
		wantHTTPS  bool
		wantReason string
	}{
		{"自动+没勾HTTPS", model.SchemeAuto, false, true, false, "只挂明文"},
		{"自动+勾了HTTPS", model.SchemeAuto, true, true, true, "两个都挂（升级前的行为）"},
		{"只要HTTP", model.SchemeHTTP, true, true, false, "勾了 HTTPS 也不挂，用户说只要 http"},
		{"只要HTTPS", model.SchemeHTTPS, false, false, true, "没勾也挂，用户说只要 https"},
	}
	for _, c := range cases {
		s := site(c.scheme, c.https)
		if got := s.WantsHTTP(); got != c.wantHTTP {
			t.Errorf("%s：WantsHTTP = %v, want %v", c.name, got, c.wantHTTP)
		}
		if got := s.WantsHTTPS(); got != c.wantHTTPS {
			t.Errorf("%s：WantsHTTPS = %v, want %v", c.name, got, c.wantHTTPS)
		}
	}
}

// 选了 https 的站点不该出现在 HTTP 入口上
func TestPlanBindingsSchemeHTTPSOnly(t *testing.T) {
	cfg := model.ProxyConfig{
		Enabled:    true,
		HTTPPorts:  []int{80},
		HTTPSPorts: []int{443},
		Sites: []model.Site{
			{ID: 1, Hosts: []string{"a.com"}, Enabled: true, Scheme: model.SchemeHTTPS},
		},
	}

	got := planBindings(cfg)
	if len(got) != 1 {
		t.Fatalf("只要 HTTPS 的站点应只开 1 个监听，实际 %d", len(got))
	}
	if !got[0].TLS || got[0].Port != 443 {
		t.Errorf("应只开 443 的 HTTPS 监听，实际 %+v", got[0])
	}
}

// 选了 http 的站点不该出现在 HTTPS 入口上
func TestPlanBindingsSchemeHTTPOnly(t *testing.T) {
	cfg := model.ProxyConfig{
		Enabled:    true,
		HTTPPorts:  []int{80},
		HTTPSPorts: []int{443},
		Sites: []model.Site{
			{ID: 1, Hosts: []string{"a.com"}, Enabled: true, Scheme: model.SchemeHTTP, HTTPS: true},
		},
	}

	got := planBindings(cfg)
	if len(got) != 1 {
		t.Fatalf("只要 HTTP 的站点应只开 1 个监听，实际 %d", len(got))
	}
	if got[0].TLS || got[0].Port != 80 {
		t.Errorf("应只开 80 的 HTTP 监听，实际 %+v", got[0])
	}
}

// 老配置的 HTTPS bool 要迁成 Scheme
func TestMigrateScheme(t *testing.T) {
	cfg := model.ProxyConfig{
		Sites: []model.Site{
			{ID: 1, HTTPS: false}, // → http
			{ID: 2, HTTPS: true},  // → auto（"两个都挂"就是自动）
		},
	}
	cfg.MigrateScheme()

	if cfg.Sites[0].Scheme != model.SchemeHTTP {
		t.Errorf("HTTPS=false 应迁成 http，实际 %q", cfg.Sites[0].Scheme)
	}
	if cfg.Sites[1].Scheme != model.SchemeAuto {
		t.Errorf("HTTPS=true 应迁成 auto，实际 %q", cfg.Sites[1].Scheme)
	}

	// 幂等：已经有 Scheme 的不该被改
	cfg.Sites[0].Scheme = model.SchemeHTTPS
	cfg.MigrateScheme()
	if cfg.Sites[0].Scheme != model.SchemeHTTPS {
		t.Errorf("已有 Scheme 不该被覆盖，实际 %q", cfg.Sites[0].Scheme)
	}
}

// 老配置里"留空域名 + 独占端口"的站点要拆成端口转发
func TestMigratePortForward(t *testing.T) {
	cfg := model.ProxyConfig{
		Sites: []model.Site{
			// 正常站点（有域名）→ 留在站点表
			{ID: 1, Hosts: []string{"a.com"}, Backend: "192.168.1.20:5000"},
			// 端口转发（没域名 + 独占端口）→ 搬走
			{ID: 2, Hosts: nil, ListenPort: 8081, Backend: "192.168.1.20:80", Enabled: true},
		},
	}

	migratePortForward(&cfg)

	if len(cfg.Sites) != 1 {
		t.Errorf("站点表应只剩 1 条，实际 %d", len(cfg.Sites))
	}
	if len(cfg.Forwards) != 1 {
		t.Fatalf("应拆出 1 条端口转发，实际 %d", len(cfg.Forwards))
	}

	f := cfg.Forwards[0]
	if f.ListenPort != 8081 || f.Target != "192.168.1.20:80" || !f.Enabled {
		t.Errorf("端口转发内容不对：%+v", f)
	}
}

// 没有端口转发时不该动（幂等 + 不误伤）
func TestMigratePortForwardNoop(t *testing.T) {
	cfg := model.ProxyConfig{
		Sites: []model.Site{
			{ID: 1, Hosts: []string{"a.com"}, Backend: "192.168.1.20:5000"},
		},
	}
	migratePortForward(&cfg)

	if len(cfg.Sites) != 1 || len(cfg.Forwards) != 0 {
		t.Errorf("没有端口转发时不该动，实际 sites=%d forwards=%d",
			len(cfg.Sites), len(cfg.Forwards))
	}

	// 已经有 Forwards 时不再拆（幂等）
	cfg2 := model.ProxyConfig{
		Sites:    []model.Site{{ID: 2, ListenPort: 8081, Backend: "x:1"}},
		Forwards: []model.Forward{{ID: 9}},
	}
	migratePortForward(&cfg2)
	if len(cfg2.Sites) != 1 || len(cfg2.Forwards) != 1 {
		t.Error("已有 Forwards 时不该再拆")
	}
}
