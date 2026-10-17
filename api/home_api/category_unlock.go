package home_api

import (
	"errors"
	"time"

	"github.com/chenggegedeid/catflap/middleware"
	"github.com/chenggegedeid/catflap/modules/home"
	"github.com/chenggegedeid/catflap/utils/pwd"
	"github.com/chenggegedeid/catflap/utils/res"

	"github.com/gin-gonic/gin"
)

type CategoryUnlockRequest struct {
	ID       string `json:"id"`
	Password string `json:"password"`
}

/**
 * CategoryUnlockView 用分组口令解锁。
 *
 * 校验顺序很有讲究：
 *   1. 先确认这个分组**确实是 lock 类型** —— 不是的话直接拒绝，
 *      不要去比对任何口令（比对本身就在告诉对方"这个分组存在且上着锁"）
 *   2. 口令错了要**限流**（见下）
 *   3. 成功才签 token
 *
 * 限流为什么放在这里：分组口令通常比登录密码简单（要给家人用），
 * 暴力试的代价更低。auth 那边是 5 次封 IP，这里复用同一套思路但轻一些 ——
 * 封的是这个浏览器会话，不牵连整个出口 IP（家里人和攻击者往往共用一个 IP）。
 */
func (HomeApi) CategoryUnlockView(c *gin.Context) {
	cr := middleware.GetBindRequest[CategoryUnlockRequest](c)

	if limited, left := unlockLimited(c, cr.ID); limited {
		res.FailRateLimitedWith(tooManyTriesMsg(left), c)
		return
	}

	var token string
	var readErr error
	home.Runtime.Read(func(cfg *home.Config) {
		for _, cat := range cfg.Categories {
			if cat.ID != cr.ID {
				continue
			}
			if !cat.NeedsUnlock() {
				readErr = home.ErrNotFound
				return
			}
			// 口令比对很慢（Argon2 故意的），只在确认这分组确实是 lock 之后才做
			matched, err := pwd.VerifyPassword(cat.PasswordHash, cr.Password)
			if err != nil {
				readErr = errors.New("口令校验繁忙，请稍后重试")
				return
			}
			if !matched {
				readErr = home.ErrBadPassword
				return
			}
			// 已解锁的保留，加上这一个 —— 免得解开第二个又锁上第一个
			ids := []string{cr.ID}
			for id := range home.VerifyUnlockToken(home.ReadUnlockToken(c)) {
				if id != cr.ID {
					ids = append(ids, id)
				}
			}
			token = home.UnlockTokenFor(ids)
			return
		}
		readErr = home.ErrNotFound
	})
	if readErr != nil {
		noteUnlockFailure(c, cr.ID)
		res.FailWithMsg(readErr.Error(), c)
		return
	}

	home.SetUnlockCookie(c, token)
	res.OkWithData(gin.H{"token": token}, c)
}

/* ===================== 尝试次数限制 ===================== */

type unlockAttempt struct {
	failures []time.Time
}

// 按 IP + 分组记，只存在内存里 —— 重启就清空，这是可接受的：
// 攻击者重启不了你的服务，而误封自己人的代价远大于让攻击点多试几次。
var unlockAttempts = map[string]*unlockAttempt{}

const (
	unlockMaxFails   = 5
	unlockWindowMins = 15
)

func attemptKey(c *gin.Context, categoryID string) string {
	ip := c.ClientIP()
	if ip == "" {
		ip = "unknown"
	}
	return ip + "|" + categoryID
}

func unlockLimited(c *gin.Context, categoryID string) (bool, int) {
	k := attemptKey(c, categoryID)
	a := unlockAttempts[k]
	if a == nil {
		return false, 0
	}
	cutoff := time.Now().Add(-unlockWindowMins * time.Minute)
	kept := a.failures[:0]
	for _, t := range a.failures {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	a.failures = kept
	if len(a.failures) < unlockMaxFails {
		return false, 0
	}
	// 还剩多久：说清才不会让人反复试
	mins := int(time.Until(a.failures[0].Add(unlockWindowMins*time.Minute)).Minutes()) + 1
	if mins < 1 {
		mins = 1
	}
	return true, mins
}

func noteUnlockFailure(c *gin.Context, categoryID string) {
	k := attemptKey(c, categoryID)
	if unlockAttempts[k] == nil {
		unlockAttempts[k] = &unlockAttempt{}
	}
	unlockAttempts[k].failures = append(unlockAttempts[k].failures, time.Now())
}

func tooManyTriesMsg(mins int) string {
	return "口令错误次数过多，请 " + itoa(mins) + " 分钟后再试"
}

func itoa(n int) string {
	if n <= 0 {
		return "0"
	}
	out := ""
	for n > 0 {
		out = string(rune('0'+n%10)) + out
		n /= 10
	}
	return out
}
