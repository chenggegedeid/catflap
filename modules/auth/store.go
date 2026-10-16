package auth

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/chenggegedeid/catflap/modules/audit"
	"github.com/chenggegedeid/catflap/utils/jwt"
	"github.com/chenggegedeid/catflap/utils/pwd"

	"github.com/sirupsen/logrus"
)

// 密码长度区间。
//
// 下限 6：加了用户名之后，攻击者要同时猜中"用户名 + 密码"，组合空间比
// 只有密码时大得多，6 位够用；再强制加长只会让用户在便签上记密码。
//
// 上限 24：**主要不是安全考虑，是防误操作** —— 挡住"把一整段文字粘进
// 密码框"这类情况（Argon2 对长输入照样算，但那种密码用户自己下次也敲不对）。
// 24 位给了足够空间，既能用口令短语（如 "correct-horse-battery"），
// 又不至于接受无限长。
//
// 注意方向：上限只在**设置新密码**时拦，登录时**不检查长度** ——
// 否则历史上设过的超长密码会直接登不进去。见 checkNewPassword 的调用点。
const (
	MinPasswordLength = 6
	MaxPasswordLength = 24
)

var (
	ErrNotInitialized   = errors.New("尚未初始化")
	ErrAlreadyInit      = errors.New("已初始化")
	ErrWrongPassword    = errors.New("密码错误")
	ErrWrongUsername    = errors.New("用户名错误")
	ErrEmptyPassword    = errors.New("密码不能为空")
	ErrPasswordTooShort = fmt.Errorf("密码至少 %d 位", MinPasswordLength)
	ErrPasswordTooLong  = fmt.Errorf("密码最多 %d 位", MaxPasswordLength)
	ErrEmptyUsername    = errors.New("用户名不能为空")
	ErrUsernameBadChar  = errors.New("用户名只能包含字母、数字、下划线和点")
	ErrPasswordBusy     = errors.New("密码校验繁忙，请稍后重试")
	ErrLoginLimited     = errors.New("登录失败次数过多，请稍后重试")
)

// checkNewPassword 设置密码和修改密码共用的检查。
// 前端也会拦一道，但接口谁都能直接调，服务端必须自己拦。
// 按字符数不按字节数，六个汉字也算六位。
func checkNewPassword(password string) error {
	if password == "" {
		return ErrEmptyPassword
	}
	n := utf8.RuneCountInString(password)
	if n < MinPasswordLength {
		return ErrPasswordTooShort
	}
	if n > MaxPasswordLength {
		return ErrPasswordTooLong
	}
	return nil
}

// checkNewUsername 设置用户名时的检查。
//
// 限制字符集而不是放开：用户名会出现在日志和审计记录里，
// 允许任意字符会带来日志注入（伪造换行插一条假的登录记录）。
func checkNewUsername(username string) error {
	u := NormalizeUsername(username)
	if u == "" {
		return ErrEmptyUsername
	}
	for _, r := range u {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '.' || r == '-'
		if !ok {
			return ErrUsernameBadChar
		}
	}
	if utf8.RuneCountInString(u) > 32 {
		return errors.New("用户名最多 32 位")
	}
	return nil
}

// WithLock 在写锁内修改配置并持久化
func (r *AuthRuntime) WithLock(mutator func(cfg *Config) error) error {
	r.mu.Lock()
	if err := mutator(&r.Config); err != nil {
		r.mu.Unlock()
		return err
	}
	snapshot := r.Config
	r.mu.Unlock()
	// 写配置是所有设置的唯一出口，所以在这里同步限流器就够了：
	// 改完密码 / 改完白名单立刻生效，不用重启。
	ApplyConfig(snapshot)
	return UpdateConfig(snapshot)
}

// Read 在读锁内读取配置
func (r *AuthRuntime) Read(reader func(cfg *Config)) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	reader(&r.Config)
}

// IsInitialized 是否已设置过密码
func (r *AuthRuntime) IsInitialized() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.Config.Initialized
}

// Setup 首次设置用户名与密码，仅在未初始化时可用。
//
// username 留空时用 DefaultUsername：老的安装流程只有密码，
// 这里不能因为没传用户名就失败。
func (r *AuthRuntime) Setup(username, password string) error {
	if err := checkNewPassword(password); err != nil {
		return err
	}
	if err := checkNewUsername(username); err != nil {
		return err
	}
	// WithLock 内仍会二次检查；此处提前返回，避免初始化后继续执行昂贵的密码哈希。
	if r.IsInitialized() {
		return ErrAlreadyInit
	}
	hash, err := pwd.HashPassword(password)
	if errors.Is(err, pwd.ErrArgon2Busy) {
		return ErrPasswordBusy
	}
	if err != nil {
		return err
	}
	return r.WithLock(func(cfg *Config) error {
		if cfg.Initialized {
			return ErrAlreadyInit
		}
		// 兜底：若 InitAuth 尚未生成 secret（启动竞态），此处补上，避免签发用空密钥
		if cfg.JwtSecret == "" {
			cfg.JwtSecret = randomSecret()
		}
		if cfg.TokenTTLHours <= 0 {
			cfg.TokenTTLHours = defaultTokenTTLHours
		}
		cfg.Username = NormalizeUsername(username)
		cfg.PasswordHash = hash
		cfg.Initialized = true
		return nil
	})
}

// Login 校验用户名和密码，成功返回签发的 token。
//
// remember 为 true 时用「记住我」的长有效期，否则用默认的短有效期。
// remoteAddr 是 TCP 对端地址（"ip:port"），用于按来源限速 —— 必须是
// Request.RemoteAddr 而不是 c.ClientIP()，见 clientKey 的说明。传空表示
// 不判断来源（只受全局总闸约束）。
//
// 用户名的兼容处理：老配置没有 Username，此时任何用户名都接受
// （用 DefaultUsername 补上）—— 否则升级后老用户会登不进去。
func (r *AuthRuntime) Login(username, password, remoteAddr string, remember bool) (string, error) {
	r.mu.RLock()
	cfg := r.Config
	r.mu.RUnlock()

	if !cfg.Initialized {
		return "", ErrNotInitialized
	}

	// 本机 + 白名单不限速、不封禁。
	//
	// 白名单用的是**审计模块那份**（审计页「永不封禁」一栏），不另开一份 ——
	// 两处各配各的，用户永远搞不清该填哪个。
	//
	// 这条是安全设施不是便利设施：没有它，别人只要知道管理员的出口 IP，
	// 故意输错几次就能把管理员自己关在门外 —— 一次成本极低的 DoS。
	var key string
	if k := clientKey(remoteAddr); !isExemptFromLimit(k) && !audit.IsWhitelisted(k) {
		key = k
	}

	// 审计的封禁列表优先：这里和「异常访问自动拉黑」共用一份，
	// 所以被自动拉黑过的来源，连登录也进不来（不是只挡页面）。
	// 也意味着上次登录失败留下的封禁**重启之后依然有效**。
	if key != "" {
		if _, blocked := audit.IsBlocked(key); blocked {
			return "", ErrLoginLimited
		}
	}

	if !globalLoginFailureLimiter.begin(key, time.Now()) {
		return "", ErrLoginLimited
	}
	loginResult := loginAttemptIgnored
	defer func() {
		// 达到阈值时把封禁写进审计：持久 + 和异常访问共用一份列表，
		// 于是审计页上看得见、标着「自动」、也能手动解封。
		if globalLoginFailureLimiter.finish(loginResult, key, time.Now()) && key != "" {
			minutes := cfg.LoginBlockMinutes
			if minutes <= 0 {
				minutes = defaultLoginBlockMinutes
			}
			reason := fmt.Sprintf("自动拉黑：登录失败 %d 次", cfg.LoginFailMax)
			if err := audit.BlockAuto(key, reason, minutes); err != nil {
				logrus.Warnf("[auth] 写入审计封禁失败（%v），进程内封禁仍然有效", err)
			}
		}
	}()

	// 用户名不对就**别去算密码哈希**：Argon2 很慢，让攻击者拿它做 CPU 消耗
	// 攻击太亏了。先判便宜的那个。
	if !usernameMatches(cfg, username) {
		loginResult = loginAttemptFailed
		return "", ErrWrongUsername
	}

	matched, err := pwd.VerifyPassword(cfg.PasswordHash, password)
	if errors.Is(err, pwd.ErrArgon2Busy) {
		return "", ErrPasswordBusy
	}
	if err != nil {
		return "", err
	}
	if !matched {
		loginResult = loginAttemptFailed
		return "", ErrWrongPassword
	}
	loginResult = loginAttemptSucceeded
	return jwt.GenerateToken(cfg.JwtSecret, tokenTTL(cfg.TokenTTLHours, remember))
}

// usernameMatches 这个用户名对不对。
//
// 老配置（Username 为空）一律放行：那种情况下"用户名"这个概念本来就不存在，
// 拿它去比对只会把升级上来的用户锁在外面。
func usernameMatches(cfg Config, username string) bool {
	if strings.TrimSpace(cfg.Username) == "" {
		return true
	}
	return subtle.ConstantTimeCompare(
		[]byte(NormalizeUsername(username)),
		[]byte(cfg.UsernameFor()),
	) == 1
}

// ValidateToken 校验 token 是否有效
func (r *AuthRuntime) ValidateToken(token string) bool {
	r.mu.RLock()
	secret := r.Config.JwtSecret
	r.mu.RUnlock()
	if secret == "" {
		return false
	}
	_, err := jwt.ParseToken(token, secret)
	return err == nil
}

// ChangePassword 校验旧密码后更新为新密码，同时换掉 JwtSecret，返回新签发的 token。
//
// newUsername 传空表示"不改用户名"（只改密码）；传了就在本次一并改掉。
//
// 换 secret 是必须的：改密码通常是因为怀疑密码泄露了，而已经发出去的 token
// 默认还能再用 7 天，不换 secret 的话拿着旧 token 的人照样进得来，改了等于没改。
// 换掉之后所有旧 token 立刻失效，返回的新 token 给当前这个浏览器续上，
// 其他设备上的登录状态会被踢掉，需要用新密码重新登录。
func (r *AuthRuntime) ChangePassword(oldPassword, newPassword, newUsername string) (string, error) {
	if err := checkNewPassword(newPassword); err != nil {
		return "", err
	}
	// 只在真的要改用户名时才校验：留空是"不改"，不该因为空值报错
	if strings.TrimSpace(newUsername) != "" {
		if err := checkNewUsername(newUsername); err != nil {
			return "", err
		}
	}
	r.mu.RLock()
	cfg := r.Config
	r.mu.RUnlock()

	if !cfg.Initialized {
		return "", ErrNotInitialized
	}
	matched, err := pwd.VerifyPassword(cfg.PasswordHash, oldPassword)
	if errors.Is(err, pwd.ErrArgon2Busy) {
		return "", ErrPasswordBusy
	}
	if err != nil {
		return "", err
	}
	if !matched {
		return "", ErrWrongPassword
	}
	hash, err := pwd.HashPassword(newPassword)
	if errors.Is(err, pwd.ErrArgon2Busy) {
		return "", ErrPasswordBusy
	}
	if err != nil {
		return "", err
	}

	var secret string
	var ttlHours int
	if err := r.WithLock(func(cfg *Config) error {
		cfg.PasswordHash = hash
		if strings.TrimSpace(newUsername) != "" {
			cfg.Username = NormalizeUsername(newUsername)
		}
		cfg.JwtSecret = randomSecret()
		if cfg.TokenTTLHours <= 0 {
			cfg.TokenTTLHours = defaultTokenTTLHours
		}
		secret, ttlHours = cfg.JwtSecret, cfg.TokenTTLHours
		return nil
	}); err != nil {
		return "", err
	}
	// 改密码后重新签发的 token 用默认（短）有效期，不走「记住我」——
	// 用户刚改完密码，正需要重新确认一次身份。
	return jwt.GenerateToken(secret, tokenTTL(ttlHours, false))
}
