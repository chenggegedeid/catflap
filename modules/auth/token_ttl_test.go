package auth

import (
	"testing"
	"time"
)

// 默认 24 小时、记住我 7 天。这条断言是刻意加的：
// 面板存着明文凭据，默认有效期一旦被改回原来的 7 天，应该有人被这行测试拦住。
func TestTokenTTL(t *testing.T) {
	cases := []struct {
		name       string
		configured int
		remember   bool
		want       time.Duration
	}{
		{"默认（未配置）24 小时", 0, false, 24 * time.Hour},
		{"记住我 7 天", 0, true, 7 * 24 * time.Hour},
		{"配置值优先于默认", 48, false, 48 * time.Hour},
		{"记住我压过配置值", 48, true, 7 * 24 * time.Hour},
		{"负数按默认处理", -5, false, 24 * time.Hour},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := tokenTTL(c.configured, c.remember); got != c.want {
				t.Fatalf("tokenTTL(%d, %v) = %v，期望 %v", c.configured, c.remember, got, c.want)
			}
		})
	}
}

func TestDefaultTokenTTLIsOneDay(t *testing.T) {
	if defaultTokenTTLHours != 24 {
		t.Fatalf("默认 token 有效期被改成了 %d 小时，确认这是有意的再改这个断言", defaultTokenTTLHours)
	}
}
