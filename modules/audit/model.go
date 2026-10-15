package audit

import (
	"time"
)

// ===================== 访问记录 =====================
//
// 和"日志"是两回事（见 docs/design/日志规范.md）：
//   - 日志是程序说"我做了什么"，文本、给人读、放 logs/
//   - 这里是**请求本身的统计**，结构化、给界面画图、放 data/
//
// 隐私取舍：**不记录 query string 和请求体**。
// URL 里可能带 token（?key=xxx 一类），而这份记录要长期保留并在界面上展示 ——
// 把凭证写进统计库是很糟的事。需要排障时看「日志」页（那里有完整 URL，
// 但不落库、按天滚动）。任何需要长期保留的东西都不该包含凭证。

// Record 一次访问
type Record struct {
	Time     time.Time `json:"time"`
	IP       string    `json:"ip"`
	Method   string    `json:"method"`
	Host     string    `json:"host"`
	Path     string    `json:"path"`
	Status   int       `json:"status"`
	BytesIn  int64     `json:"bytesIn"`
	BytesOut int64     `json:"bytesOut"`
	// DurationMs 耗时（毫秒）。慢请求是另一个值得关注的点
	DurationMs int64  `json:"durationMs"`
	UserAgent  string `json:"userAgent"`
	SiteID     uint   `json:"siteId"`
}

// LoginEvent 一次登录尝试。
//
// **单独一个类型而不是塞进 Record**：登录不是"访问某个站点"，它没有
// 域名、路径、字节数。硬塞进去会让每条记录都带着几个空字段，
// 界面上也得靠"这些字段是不是空"来分辨，很别扭。
//
// **绝不记录密码本身** —— 连掩码都不记。审计记录要长期保留并在界面上展示，
// 里面出现任何密码片段都是事故。
type LoginEvent struct {
	Time time.Time `json:"time"`
	IP   string    `json:"ip"`
	// Username 试的用户名。记它是为了看"攻击者在猜哪些名字"
	// （比如看到一堆 admin/root/test 就知道有人在字典攻击）
	Username string `json:"username"`
	// Success 是否成功
	Success bool `json:"success"`
	// Reason 失败原因：密码错误 / 次数超限 / 尚未初始化
	Reason string `json:"reason,omitempty"`
	// UserAgent 浏览器标识
	UserAgent string `json:"userAgent,omitempty"`
}

// isError 4xx/5xx 都算异常访问
func (r Record) isError() bool { return r.Status >= 400 }

// ===================== IP 统计 =====================

// IPStat 按来源 IP 聚合的统计。给「IP 管理」页用。
type IPStat struct {
	IP        string    `json:"ip"`
	FirstSeen time.Time `json:"firstSeen"`
	LastSeen  time.Time `json:"lastSeen"`
	Requests  int64     `json:"requests"`
	BytesIn   int64     `json:"bytesIn"`
	BytesOut  int64     `json:"bytesOut"`
	// Errors 4xx/5xx 计数。扫端口、猜路径的请求绝大多数落在 404，
	// 这个数字是识别扫描器最直接的信号。
	Errors int64 `json:"errors"`
	// Paths 访问过的路径（去重，最多留 N 个）。
	// 看它扫的是什么就能判断意图：/wp-admin 是打 WordPress，/.env 是找泄露的配置。
	Paths []string `json:"paths"`
	// Hosts 访问过哪些域名（去重，最多留少量）
	Hosts []string `json:"hosts"`
	// UserAgents 见过的 UA（去重，最多几个）。扫描器的 UA 往往很特征
	UserAgents []string `json:"userAgents"`

	// Blocked 是否在全局封禁表里
	Blocked bool `json:"blocked"`
	// BlockReason 封禁原因（手动填的说明，或自动判定）
	BlockReason string `json:"blockReason"`
	// BlockExpiresAt 临时封禁的到期时间；零值 = 永久
	BlockExpiresAt time.Time `json:"blockExpiresAt,omitempty"`
}

// Suspicious 是不是"值得处理"的 IP。
//
// 判据：错误数占比高 **且** 有一定量级。只看错误数会把正常服务的
// 4xx（比如接口被正常调用但参数不对）也算进来，看占比更准。
//
// 为什么要有这个方法而不是让界面自己判：判据是产品逻辑（"什么叫可疑"），
// 放在界面里改不动、也测不了。
func (s IPStat) Suspicious() bool {
	// 量级门槛：太少的样本不说明问题（自己点两下也可能 100% 是 4xx）
	if s.Errors < minSuspiciousErrors {
		return false
	}
	if s.Requests == 0 {
		return false
	}
	return float64(s.Errors)/float64(s.Requests) >= suspiciousErrorRatio
}

const (
	// minSuspiciousErrors 至少这么多次错误才谈得上"可疑"
	minSuspiciousErrors = 50
	// suspiciousErrorRatio 错误占比达到这个值才可疑
	suspiciousErrorRatio = 0.8
)

// ===================== 全局封禁 =====================

// BlockEntry 一条封禁记录
type BlockEntry struct {
	IP      string    `json:"ip"`
	Reason  string    `json:"reason"`
	AddedAt time.Time `json:"addedAt"`
	// ExpiresAt 到期时间。零值 = 永久。
	ExpiresAt time.Time `json:"expiresAt,omitempty"`
	// Auto 是自动拉黑的（区别于手动）。界面上要能区分 ——
	// 自动的可能误伤，用户看到来源才敢放心删。
	Auto bool `json:"auto"`
}

// Expired 这条封禁是否已过期
func (b BlockEntry) Expired(now time.Time) bool {
	return !b.ExpiresAt.IsZero() && now.After(b.ExpiresAt)
}

// Config 审计与封禁的配置
type Config struct {
	// Enabled 是否记录访问。默认开 —— 不记录就没法查"谁在扫我"。
	Enabled bool `json:"enabled"`
	// KeepDays 明细保留天数
	KeepDays int `json:"keepDays"`
	// MaxFileMB 单天文件上限（MB）。超了停止写入并告警 ——
	// 被刷的时候最怕的不是请求本身，是把磁盘写满导致服务挂掉。
	MaxFileMB int `json:"maxFileMB"`

	// AutoBlock 自动拉黑。**默认关**：
	// 它容易误伤（自己调试时刷出几百个 404 就把自己封了）。
	AutoBlock bool `json:"autoBlock"`
	// AutoBlockThreshold 自动拉黑的错误数门槛
	AutoBlockThreshold int64 `json:"autoBlockThreshold"`
	// AutoBlockMinutes 自动封禁时长（分钟）。0 = 永久
	AutoBlockMinutes int `json:"autoBlockMinutes"`
	// Whitelist 永不封禁的 IP/网段。开了自动拉黑时它是兜底 ——
	// 别把管理员自己关在外面。
	Whitelist []string `json:"whitelist"`

	// TrustedProxies 可信反代。配了之后会从 X-Forwarded-For 取真实来源。
	//
	// **默认空**：不配就只用 TCP 对端地址（伪造不了）。
	// 配了就意味着"相信这个来源写的转发头"，只该在确实挡了 Cloudflare
	// 之类反代时填。填错会让封禁形同虚设（谁都能伪造来源）。
	TrustedProxies []string `json:"trustedProxies"`

	Blocks []BlockEntry `json:"blocks"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
