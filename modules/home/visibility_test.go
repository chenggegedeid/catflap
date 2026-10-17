package home

import (
	"testing"
	"time"
)

func TestMigrateVisibilityFromRequireAuth(t *testing.T) {
	cfg := &Config{
		SchemaVersion: LayoutSchemaV2,
		Categories: []Category{
			{ID: "c1", Name: "公开", RequireAuth: false},
			{ID: "c2", Name: "私密", RequireAuth: true},
		},
	}
	if !Migrate(cfg) {
		t.Fatal("V2 配置应当触发可见性迁移")
	}
	if cfg.Categories[0].Visibility != VisibilityPublic {
		t.Errorf("没勾 requireAuth 的应当保持公开，实际 %q", cfg.Categories[0].Visibility)
	}
	if cfg.Categories[1].Visibility != VisibilityLogin {
		t.Errorf("勾了的应当是 login，实际 %q", cfg.Categories[1].Visibility)
	}
	if cfg.SchemaVersion != VisibilitySchemaV3 {
		t.Errorf("schemaVersion = %d，期望 %d", cfg.SchemaVersion, VisibilitySchemaV3)
	}
}

// 迁移必须幂等
func TestMigrateVisibilityIsIdempotent(t *testing.T) {
	cfg := &Config{
		SchemaVersion: LayoutSchemaV2,
		Categories:    []Category{{ID: "c1", RequireAuth: true}},
	}
	Migrate(cfg)
	if Migrate(cfg) {
		t.Error("第二次不该再改")
	}
	if cfg.Categories[0].Visibility != VisibilityLogin {
		t.Errorf("第二次跑完可见性变了：%q", cfg.Categories[0].Visibility)
	}
}

// 认不出的可见性按 public 处理 —— 宁可漏藏，不可误藏
func TestNormalizeVisibilityFallsBackToPublic(t *testing.T) {
	cases := map[string]string{
		"":          VisibilityPublic,
		"public":    VisibilityPublic,
		"login":     VisibilityLogin,
		"lock":      VisibilityLock,
		"wat":       VisibilityPublic,
		"PUBLIC":    VisibilityPublic, // 大小写敏感，不认识的回落公开
		"login\x00": VisibilityPublic,
	}
	for in, want := range cases {
		if got := NormalizeVisibility(in); got != want {
			t.Errorf("NormalizeVisibility(%q) = %q，期望 %q", in, got, want)
		}
	}
}

/* ===================== 解锁 token ===================== */

func TestUnlockTokenRoundTrip(t *testing.T) {
	token := UnlockTokenFor([]string{"c2", "c1"})
	got := VerifyUnlockToken(token)
	if !got["c1"] || !got["c2"] {
		t.Errorf("签发的两个分组都该在解锁集合里，实际 %v", got)
	}
}

func TestUnlockTokenRejectsTampering(t *testing.T) {
	token := UnlockTokenFor([]string{"c1"})
	// 改一个字符：签名对不上就应当拒绝
	tampered := token[:len(token)-1] + "x"
	if got := VerifyUnlockToken(tampered); got != nil {
		t.Errorf("被改过的 token 不该通过，实际放行了 %v", got)
	}
}

func TestUnlockTokenRejectsExpired(t *testing.T) {
	// 伪造一个已经过期的时间戳（签名仍用真密钥不行，所以这里验的是"过期"分支：
	// 直接构造一个签名正确但过期的 token）
	expires := time.Now().Add(-time.Hour)
	payload := "c1|" + itoa64(expires.Unix())
	mac := hmacOf(payload)
	if got := VerifyUnlockToken(payload + "|" + mac); got != nil {
		t.Error("过期的 token 不该通过")
	}
}

func TestUnlockTokenRejectsGarbage(t *testing.T) {
	for _, bad := range []string{"", "abc", "a|b", "a|b|c|d"} {
		if got := VerifyUnlockToken(bad); got != nil {
			t.Errorf("%q 不该通过，实际放行了 %v", bad, got)
		}
	}
}
