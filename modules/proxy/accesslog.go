package proxy

import (
	"bufio"
	"net"
	"net/http"
	"time"

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
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

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
			traffic.Record(siteID, bytesIn, rec.bytesOut, rec.status)
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
// 反代自己就是最外面那一层，RemoteAddr 就是对端——
// 所以不看 X-Forwarded-For（那是客户端能随便伪造的）。
func clientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// statusRecorder 拿状态码 + 出站字节数。
//
// 不能简单套一层就算完：WebSocket 要 Hijack，SSE 要 Flush，
// 少实现一个接口就会把功能打掉——这正是「加访问日志加坏了反代」的经典原因。
//
// 出站字节在这里数：它是**唯一**能拿到"实际写给客户端的字节"的位置，
// 反代的 Rewrite 阶段拿不到最终大小（有压缩、有分块）。
type statusRecorder struct {
	http.ResponseWriter
	status   int
	written  bool
	bytesOut int64
}

func (s *statusRecorder) WriteHeader(code int) {
	if !s.written {
		s.status = code
		s.written = true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	s.written = true
	n, err := s.ResponseWriter.Write(b)
	// 只加真的写出去的字节数：Write 返回 n < len(b) 时说明只写了一部分
	s.bytesOut += int64(n)
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
