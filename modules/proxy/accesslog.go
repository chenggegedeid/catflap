package proxy

import (
	"bufio"
	"net"
	"net/http"
	"time"

	"github.com/chenggegedeid/catflap/modules/audit"
	"github.com/chenggegedeid/catflap/modules/traffic"

	"github.com/sirupsen/logrus"
)

// withAccessLog 访问日志 + 流量采集包装。
//
// 两件事共用一层包装（都要 statusRecorder，套两层会多一次接口转发），
// 但**开关是独立的**：
//   - 访问日志默认关：家里就这么几个服务，日志刷屏比没日志更烦人
//   - 流量采集默认开：统计不占地方也不刷屏，用户装了就想看到数据
//
// 日志走 logrus（会被收集进运行日志文件，见 core/logrus.go），
// 流量走 traffic 模块落盘成结构化数据。
func withAccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logging := Runtime.accessLogEnabled()
		if !logging && !traffic.Enabled() {
			next.ServeHTTP(w, r)
			return
		}

		// 站点 ID 的容器要在这里创建：站点是里面 serve() 匹配出来的，
		// 而我们要在它返回之后才统计 —— 得有个两边都能摸到的东西。
		r = traffic.NewSiteRef(r)

		start := time.Now()
		rec := &statusRecorder{
			ResponseWriter: w,
			status:         http.StatusOK,
			// 站点 ID 是里面 serve() 匹配出来之后才写进 SiteRef 的，
			// 而我们要在**响应写入途中**就上报 —— 所以给一个读取函数，
			// 而不是在创建时拷一个值（那一刻它还是 0）。
			siteIDOf: func() uint { return traffic.SiteIDFromRequest(r) },
		}

		// 请求体大小：反代读的是 r.Body，要包一层才数得到。
		// 有些请求没有 body（GET），ContentLength 会是 -1，按 0 处理。
		bytesIn := r.ContentLength
		if bytesIn < 0 {
			bytesIn = 0
		}

		next.ServeHTTP(rec, r)

		// 统计要站点 ID 才能按服务分组；反代在 serve 里写进去的
		siteID := traffic.SiteIDFromRequest(r)

		if logging {
			logrus.Infof("[proxy] %s %s %s%s → %d %s (%s) ↑%s ↓%s",
				clientIP(r), r.Method, r.Host, r.URL.RequestURI(),
				rec.status, r.Proto, time.Since(start).Round(time.Millisecond),
				traffic.HumanBytes(bytesIn), traffic.HumanBytes(rec.bytesOut))
		}

		if traffic.Enabled() {
			// 结算：把最后一段还没上报的字节补上（传输途中已分段上报过）。
			//
			// 必须只补**差值**：途中已经报过 rec.reported 字节，
			// 这里再传完整的 rec.bytesOut 就会重复计一次，流量凭空翻倍。
			//
			// 请求数在这里记（一次下载 = 一个请求），字节只补剩余部分。
			remaining := rec.bytesOut - rec.reported
			if remaining < 0 {
				remaining = 0 // 理论上不会发生，兜住避免负数污染累计值
			}
			traffic.Record(siteID, bytesIn, remaining, rec.status)
			rec.reported = rec.bytesOut
		}

		// 访问审计：记结构化记录（与日志独立，见 docs/design/日志规范.md）。
		// **不记 query string** —— URL 里可能带 token，而这份记录要长期保留。
		//
		// 但**被自己封禁拒绝的请求不记**：那个 403 是我们发的，不是对方的问题。
		// 记进去会让被拉黑 IP 的错误数继续涨（而它已经封了），
		// 统计上看着像"封了还在扫"，其实是自己数自己。
		if audit.Enabled() && !rec.denied {
			audit.RecordAccess(audit.Record{
				Time:       start,
				IP:         clientIP(r),
				Method:     r.Method,
				Host:       r.Host,
				Path:       r.URL.Path,
				Status:     rec.status,
				BytesIn:    bytesIn,
				BytesOut:   rec.bytesOut,
				DurationMs: time.Since(start).Milliseconds(),
				UserAgent:  r.UserAgent(),
				SiteID:     siteID,
			})
		}
	})
}

func (r *ProxyRuntime) accessLogEnabled() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.Config.AccessLog
}

// clientIP 记真实来访者。
//
// 判断统一交给 audit.ResolveClientIP，不在这里自己写一份：
// 封禁（handler.go）、访问日志、审计记录三处调的都是这个函数，
// 它们**必须看到同一个 IP**，否则会出现「封禁按对端地址判、审计按转发头记」
// 这种表面生效、统计却还在涨的情况。
//
// 默认只信 TCP 对端（RemoteAddr）—— 伪造 X-Forwarded-For 没用；
// 只有把来源配进审计设置里的「可信反代」后，才会去 XFF 里找真实来源
// （走 Cloudflare 时对端是 CF，那是唯一能拿到真实来源的地方）。
func clientIP(r *http.Request) string {
	return audit.ResolveClientIP(r)
}

// statusRecorder 拿状态码 + 出站字节数。
//
// 不能简单套一层就算完：WebSocket 要 Hijack，SSE 要 Flush，
// 少实现一个接口就会把功能打掉——这正是「加访问日志加坏了反代」的经典原因。
//
// 出站字节在这里数：它是**唯一**能拿到"实际写给客户端的字节"的位置，
// 反代的 Rewrite 阶段拿不到最终大小（有压缩、有分块）。
// traffic 途中上报的两个阈值：够了哪个都上报一次。
//
// # 为什么定这两个数
//
//   - 字节阈值（4MB）：大流量时按量上报。20Mbps 的下载约 2 秒到 4MB，
//     速率曲线和"今日流量"每两秒动一次，肉眼跟得上。
//   - 时间阈值（5 秒）：照顾**慢速长传**。一个 200KB/s 的下载要 20 秒才攒够 4MB，
//     只有字节阈值的话，这 20 秒里统计是空的 —— 正是用户看到的"没效果"。
//
// 上报一次只是一次 atomic + 一次短锁，代价可以忽略；反过来漏掉一段
// 就是"统计不准"的观感，所以宁可报得勤一点。
const (
	progressReportBytes    = 4 << 20 // 4 MB
	progressReportInterval = 5 * time.Second
)

type statusRecorder struct {
	http.ResponseWriter
	status   int
	written  bool
	bytesOut int64
	// denied 这个响应是"门禁拒绝"（全局封禁 / 站点白黑名单 / 无匹配站点）。
	// 审计要跳过它们，理由见 withAccessLog 里的注释。
	denied bool

	// 下面三个是「途中上报」用的状态。
	//
	// reported 已经上报过的字节数；每次 Write 后看 bytesOut-reported 够不够阈值，
	// 够了就把增量交给 traffic.RecordPartial。请求结束时再把剩余那段补齐，
	// 这样既不会漏也不会重复计数。
	reported   int64
	lastReport time.Time
	// siteIDOf 读当前站点 ID。延迟到用的时候再读，因为 serve() 是在
	// ResponseWriter 创建之后才匹配到站点并写进 SiteRef 的。
	siteIDOf func() uint
}

// reportProgressLocked 把自上次上报以来新增的字节交给流量统计。
//
// 注意 requests 传 0：一次下载是一个请求，不能因为分段上报就变成好几个。
func (s *statusRecorder) reportProgress(force bool) {
	pending := s.bytesOut - s.reported
	if pending <= 0 {
		return
	}
	if !force && pending < progressReportBytes && time.Since(s.lastReport) < progressReportInterval {
		return
	}
	traffic.RecordPartial(s.siteID(), 0, pending)
	s.reported = s.bytesOut
	s.lastReport = time.Now()
}

func (s *statusRecorder) siteID() uint {
	if s.siteIDOf == nil {
		return 0
	}
	return s.siteIDOf()
}

// markDenied 从 ResponseWriter 反查我们那层包装并打标记。
//
// 门禁的响应函数拿到的 w 可能是被包过的（中间还可能有别的包装），
// 所以用 Unwrap 一路找下去 —— 直接断言会漏掉。
func markDenied(w http.ResponseWriter) {
	for i := 0; i < 8 && w != nil; i++ {
		if rec, ok := w.(*statusRecorder); ok {
			rec.markDenied()
			return
		}
		u, ok := w.(interface{ Unwrap() http.ResponseWriter })
		if !ok {
			return
		}
		w = u.Unwrap()
	}
}

// markDenied 标记这个响应是门禁拒绝的。
// 门禁的写响应函数调它 —— 用显式标记而不是靠比对状态码，
// 因为 403 也可能是后端服务自己返回的（那种要记）。
func (s *statusRecorder) markDenied() { s.denied = true }

func (s *statusRecorder) WriteHeader(code int) {
	if !s.written {
		s.status = code
		s.written = true
	}
	s.ResponseWriter.WriteHeader(code)
}

// Write 数出站字节，并在传输途中分段上报流量统计。
//
// # 为什么不能只在最后记一次
//
// 见 traffic.RecordPartial 的注释：大文件下载要传十几分钟，
// 只在请求结束时记一次的话，这十几分钟里仪表盘、实时速率、今日流量
// 全都是零 —— 用户在路由器上明明看到上行在跑。
// 所以在写入过程中就按阈值上报，结束时再补最后一段。
func (s *statusRecorder) Write(b []byte) (int, error) {
	s.written = true
	n, err := s.ResponseWriter.Write(b)
	// 只加真的写出去的字节数：Write 返回 n < len(b) 时说明只写了一部分
	s.bytesOut += int64(n)
	s.reportProgress(false)
	return n, err
}

func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := s.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	s.status = http.StatusSwitchingProtocols
	return h.Hijack()
}

// Unwrap 让 http.ResponseController 能拿到真身，
// SetReadDeadline / SetWriteDeadline 这类调用才不会被这层包装挡住
func (s *statusRecorder) Unwrap() http.ResponseWriter {
	return s.ResponseWriter
}
