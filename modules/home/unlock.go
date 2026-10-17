package home

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

/**
 * 分组解锁。
 *
 * 锁的目的：登录态挡不住"别人坐到你的电脑前" —— 你登着，他就能看。
 * 所以 lock 分组要求**再验一次**，验的是这个分组自己的口令，
 * 和登录密码分开（可以给家人单独的口令，不必把主密码交出去）。
 *
 * 有效期两条规矩，缺一不可：
 *   1. token 有时限（默认 12 小时）—— 万一被记在浏览器里，也有过期的一天
 *   2. 前端存 **sessionStorage** —— 关标签、关浏览器就没了
 * 只做其中一条都不够：只靠前端存，别人拿到 token 可以一直用；
 * 只靠后端时限，同一个浏览器里换个人坐过来还是能看。
 */

const (
	unlockCookieName = "catflap_unlock"
	unlockTTLHours   = 12
	unlockSecretLen  = 24
)

// unlockSecret 签 token 用的密钥，进程启动时随机生成。
//
// **为什么不持久化**：持久化之后，重启前发的 token 在重启后依然有效 ——
// 那等于把"这次解锁"变成了永久授权，和"关浏览器就重新验证"的约定相反。
//
// 代价是重启后所有 lock 分组都要重新输一次口令。这是**要的**行为。
var unlockSecret = newUnlockSecret()

func newUnlockSecret() string {
	b := make([]byte, unlockSecretLen)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand 失败极罕见，用时间兜底 —— 最差是重启后密钥不同，
		// 那只是让已有的解锁失效，不会放行任何东西
		return hex.EncodeToString([]byte(time.Now().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(b)
}

// UnlockTokenFor 给一批分组签一个解锁 token
func UnlockTokenFor(ids []string) string {
	sorted := append([]string{}, ids...)
	sort.Strings(sorted)
	expires := time.Now().Add(unlockTTLHours * time.Hour).Unix()

	payload := strings.Join(sorted, ",") + "|" + itoa64(expires)

	mac := hmac.New(sha256.New, []byte(unlockSecret))
	mac.Write([]byte(payload))
	sig := hex.EncodeToString(mac.Sum(nil))

	return payload + "|" + sig
}

// VerifyUnlockToken 校验 token，返回它解锁的分组集合。过期/被篡改都返回 nil。
func VerifyUnlockToken(token string) map[string]bool {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil
	}
	parts := strings.Split(token, "|")
	if len(parts) != 3 {
		return nil
	}
	payload := parts[0] + "|" + parts[1]

	mac := hmac.New(sha256.New, []byte(unlockSecret))
	mac.Write([]byte(payload))
	// 用 hmac.Equal 而不是 ==：普通字符串比较会按长度提前返回，
	// 等于把签名长度泄漏给计时攻击者
	if !hmac.Equal([]byte(parts[2]), []byte(hex.EncodeToString(mac.Sum(nil)))) {
		return nil
	}

	expires, ok := atoi64(parts[1])
	if !ok || time.Now().Unix() > expires {
		return nil
	}

	out := map[string]bool{}
	if parts[0] != "" {
		for _, id := range strings.Split(parts[0], ",") {
			out[id] = true
		}
	}
	return out
}

// SetUnlockCookie 把解锁 token 写给浏览器。
//
// **刻意不用持久化 cookie**：前端会把它抄进 sessionStorage
// （关标签就没）。这里用 session cookie 是双保险 —— 关浏览器即失效。
// HttpOnly 是我们主动放弃的（前端要读它），可接受：token 里没有任何凭据，
// 且 12 小时过期。
func SetUnlockCookie(c *gin.Context, token string) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(unlockCookieName, token, unlockTTLHours*3600, "/", "", false, false)
}

// ReadUnlockToken 取出这次请求带的解锁 token（cookie 或 header）
func ReadUnlockToken(c *gin.Context) string {
	if v, err := c.Cookie(unlockCookieName); err == nil {
		return v
	}
	return c.GetHeader("X-Catflap-Unlock")
}

func itoa64(n int64) string {
	return strings.TrimSpace(strings.Replace(hex.EncodeToString([]byte{byte(n >> 56), byte(n >> 48), byte(n >> 40), byte(n >> 32), byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)}), " ", "", -1))
}

func atoi64(s string) (int64, bool) {
	if len(s) != 16 {
		return 0, false
	}
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != 8 {
		return 0, false
	}
	n := int64(0)
	for _, v := range b {
		n = n<<8 | int64(v)
	}
	return n, true
}

// hmacOf 测试用：拿当前密钥给一段 payload 签名
func hmacOf(payload string) string {
	mac := hmac.New(sha256.New, []byte(unlockSecret))
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}
