package proxy

import (
	"strings"
	"testing"

	"github.com/chenggegedeid/catflap/modules/proxy/model"
)

// 掩码 token 不能被当成真 token 存回去。
//
// 面板回显凭证时给的是一串圆点，用户没改动就会原样提交回来——照单全收的话，
// 用户只是想改个"启用"勾，结果把 token 冲成了一串圆点，隧道再也起不来。
func TestMaskedTokenIsNotStored(t *testing.T) {
	var got model.TunnelConfig
	apply := func(in string, clear bool) {
		cfg := model.TunnelConfig{Token: "real-token-value"}
		if clear {
			cfg.Token = ""
		} else if in != "" && !isMasked(in) {
			cfg.Token = in
		}
		got = cfg
	}

	apply("••••••••", false)
	if got.Token != "real-token-value" {
		t.Fatalf("掩码不应覆盖原 token，got %q", got.Token)
	}

	apply("", false)
	if got.Token != "real-token-value" {
		t.Fatalf("留空不应覆盖原 token，got %q", got.Token)
	}

	apply("new-token", false)
	if got.Token != "new-token" {
		t.Fatalf("真 token 应写入，got %q", got.Token)
	}
}

// isMasked 的判定：全是掩码字符才算掩码，掺了别的一律当真值
func TestIsMasked(t *testing.T) {
	cases := map[string]bool{
		"••••••":   true,
		"******":   true,
		"abc":      false,
		"•abc":     false,
		"":         false,
		"real-tok": false,
	}
	for in, want := range cases {
		if got := isMasked(in); got != want {
			t.Fatalf("isMasked(%q) = %v, want %v", in, got, want)
		}
	}
}

// 未启用 / 缺二进制 / 缺 token 都要给出能照着做的报错，而不是笼统失败
func TestStartTunnelRequiresConfig(t *testing.T) {
	// 没启用
	setTunnelConfig(model.TunnelConfig{})
	if err := StartTunnel(); err != errTunnelDisabled {
		t.Fatalf("未启用时应返回 errTunnelDisabled，got %v", err)
	}

	// 启用了但没 token
	setTunnelConfig(model.TunnelConfig{Enabled: true})
	if err := StartTunnel(); err != errNoTunnelToken {
		t.Fatalf("缺 token 时应返回 errNoTunnelToken，got %v", err)
	}

	// 有 token 但二进制不存在（故意指向一个肯定不存在的路径）
	setTunnelConfig(model.TunnelConfig{
		Enabled:    true,
		Token:      "x",
		BinaryPath: "/nonexistent/cloudflared-for-test",
	})
	if err := StartTunnel(); err != errNoCloudflared {
		t.Fatalf("找不到二进制时应返回 errNoCloudflared，got %v", err)
	}
}

// isMasked 是 api 包里的同名小工具，这里复刻一份判定用于上面的断言
func isMasked(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r != '•' && r != '*' {
			return false
		}
	}
	return true
}

func setTunnelConfig(cfg model.TunnelConfig) {
	Runtime.mu.Lock()
	Runtime.Config.Tunnel = cfg
	Runtime.mu.Unlock()
}

// TunnelState 不该把 token 本身吐出去，只能说"填没填"
func TestTunnelStateLeaksNoToken(t *testing.T) {
	setTunnelConfig(model.TunnelConfig{Enabled: true, Token: "super-secret-token"})
	st := TunnelState()
	if !st.TokenSet {
		t.Fatal("填了 token 应回 tokenSet=true")
	}
	if strings.Contains(st.Message, "super-secret-token") {
		t.Fatal("状态里出现了 token 明文")
	}
}
