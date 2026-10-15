package auth

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// 用户名一律按小写存、按小写比 —— 区分大小写只会让用户自己登不上
func TestUsernameNormalization(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Admin", "admin"},
		{"  Admin  ", "admin"},
		{"ADMIN", "admin"},
		{"my.user-1_x", "my.user-1_x"},
	}
	for _, c := range cases {
		if got := NormalizeUsername(c.in); got != c.want {
			t.Errorf("NormalizeUsername(%q) = %q, want %q", c.in, got, c.want)
		}
	}

	// 留空回落默认值
	if got := NormalizeUsernameDefault(""); got != DefaultUsername {
		t.Errorf("空用户名应回落 %q，实际 %q", DefaultUsername, got)
	}
	if got := NormalizeUsernameDefault("Root"); got != "root" {
		t.Errorf("NormalizeUsernameDefault(Root) = %q, want root", got)
	}
}

// **关键的兼容行为**：老配置没有 Username，此时任何用户名都得能登进去。
//
// 否则升级后老用户会被锁在外面 —— 那种安装里"用户名"这个概念根本不存在。
func TestLegacyConfigWithoutUsernameAcceptsAny(t *testing.T) {
	cfg := Config{Initialized: true, PasswordHash: "x"} // 没有 Username

	for _, u := range []string{"admin", "root", "whatever", "", "ADMIN"} {
		if !usernameMatches(cfg, u) {
			t.Errorf("老配置下用户名 %q 应该一律通过", u)
		}
	}

	// 但 UsernameFor 仍要给一个有意义的默认值（界面显示用）
	if cfg.UsernameFor() != DefaultUsername {
		t.Errorf("老配置的 UsernameFor 应为 %q，实际 %q", DefaultUsername, cfg.UsernameFor())
	}
}

// 设过用户名之后就要严格比对（大小写不敏感）
func TestUsernameMatchesAfterSet(t *testing.T) {
	cfg := Config{Initialized: true, Username: "nasadmin"}

	if !usernameMatches(cfg, "nasadmin") {
		t.Error("正确用户名应通过")
	}
	if !usernameMatches(cfg, "NASADMIN ") { // 大小写 + 空格都该容忍
		t.Error("用户名大小写与空格应被容忍")
	}
	if usernameMatches(cfg, "other") {
		t.Error("错误用户名应被拒")
	}
	if usernameMatches(cfg, "") {
		t.Error("空用户名应被拒")
	}
}

// 用户名字符集要限制：它会进日志和审计记录，
// 放开任意字符会被用来伪造日志行（插入换行造一条假的登录记录）
func TestUsernameCharsetRestricted(t *testing.T) {
	bad := []string{
		"admin\nINFO 登录成功", // 日志注入
		"admin\r\nfake",
		"admin\x00",
		"用户名", // 非 ASCII
		"a b", // 含空格
	}
	for _, u := range bad {
		if err := checkNewUsername(u); err == nil {
			t.Errorf("用户名 %q 不该被接受", u)
		}
	}

	good := []string{"admin", "nas.admin", "user_1", "user-1", "A"}
	for _, u := range good {
		if err := checkNewUsername(u); err != nil {
			t.Errorf("用户名 %q 应被接受：%v", u, err)
		}
	}

	// 空用户名报错（不是静默接受）
	if err := checkNewUsername(""); err != ErrEmptyUsername {
		t.Errorf("空用户名应返回 ErrEmptyUsername，实际 %v", err)
	}
}

// 密码长度区间 6~12，按字符数不按字节数
func TestPasswordLengthBounds(t *testing.T) {
	if MinPasswordLength != 6 || MaxPasswordLength != 24 {
		t.Fatalf("长度区间应为 6~24，实际 %d~%d", MinPasswordLength, MaxPasswordLength)
	}

	// 下限
	if err := checkNewPassword(strings.Repeat("a", MinPasswordLength-1)); err != ErrPasswordTooShort {
		t.Errorf("短于下限应返回 ErrPasswordTooShort，实际 %v", err)
	}
	if err := checkNewPassword(strings.Repeat("a", MinPasswordLength)); err != nil {
		t.Errorf("刚好够下限不该报错：%v", err)
	}
	// 上限
	if err := checkNewPassword(strings.Repeat("a", MaxPasswordLength)); err != nil {
		t.Errorf("刚好在上限不该报错：%v", err)
	}
	if err := checkNewPassword(strings.Repeat("a", MaxPasswordLength+1)); err != ErrPasswordTooLong {
		t.Errorf("超过上限应返回 ErrPasswordTooLong，实际 %v", err)
	}

	// 汉字按字符算：6 个汉字 = 6 位
	six := "六个汉字密码"
	if n := utf8.RuneCountInString(six); n != MinPasswordLength {
		t.Fatalf("测试字符串应为 %d 个字符，实际 %d", MinPasswordLength, n)
	}
	if err := checkNewPassword(six); err != nil {
		t.Errorf("6 个汉字应该算 6 位：%v", err)
	}

	// 空密码
	if err := checkNewPassword(""); err != ErrEmptyPassword {
		t.Errorf("空密码应返回 ErrEmptyPassword，实际 %v", err)
	}
}
