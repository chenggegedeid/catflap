package stun

import (
	"crypto/tls"
	"errors"
	"io"
	"net"
	"sync"
	"time"

	"github.com/chenggegedeid/catflap/modules/plainhttp"
	"github.com/chenggegedeid/catflap/modules/traffic"
	"github.com/chenggegedeid/catflap/utils/nettune"

	"github.com/sirupsen/logrus"
)

// tlsHandshakeTimeout 握手必须有超时兜底：
// 不带 TLS 时连接一进来就立刻拨号，而 TLS 握手要等对端先说话，
// 端口扫描器开个连接不发任何数据就能钉住一个 goroutine。
//
// 用 var 而不是 const，只为让测试能把它调短——运行时没有任何地方改它。
var tlsHandshakeTimeout = 10 * time.Second

// ForwardOptions 单个 STUN 洞的转发选项
type ForwardOptions struct {
	// TLSConfig 非 nil 时由 Catflap 在洞口终结 TLS。
	// 一个洞固定对应一个后端服务，身份由端口决定，因此不需要解析 HTTP——
	// 握手完成后继续按字节流转发即可，WebSocket / SSE / HTTP2 天然透传。
	TLSConfig *tls.Config

	// BackendHTTPS 拨内网时用 tls.Dial 而不是裸 dial，等价于 nginx 的 proxy_pass https://。
	//
	// 注意它不是「内网是不是 HTTPS」的描述，而是「Catflap 要不要自己去和内网握手」，
	// 只在 TLSConfig != nil（洞口终结了 TLS、Catflap 真的要重新建一条连接）时才成立。
	// TLSConfig 为 nil 时这个洞是纯字节管道，握手是浏览器和内网服务之间的事，
	// 此时再置 true 就成了双层 TLS——浏览器的 ClientHello 被当明文塞进 Catflap
	// 自己那条 TLS，必然握手失败。配置层（normalizeBackendHTTPS / normalizeServices）
	// 已经挡掉这个组合，这里保持诚实：给什么就做什么。
	BackendHTTPS bool
}

// ForwardTCP 把洞上的连接转发到内网服务
//
// deviceID / serviceID 只用于流量统计归组，填 0 也能正常转发
// （统计会归到"未分组"，转发行为完全不受影响）。
func ForwardTCP(src net.Conn, targetAddr string, protocol string, opt ForwardOptions, deviceID, serviceID uint) {
	defer src.Close()

	// ★ 吞吐调优，且**必须在 TLS 握手之前**：
	//
	// 下面 opt.TLSConfig != nil 时 src 会被换成 *tls.Conn，那时候断言不到
	// *net.TCPConn 了。这里是唯一能摸到底层 TCP 连接的时机。
	//
	// 为什么必须调：见 utils/nettune 的说明。穿透这条路的延迟比内网还高一层
	// （要经 NAT 映射到公网），窗口不够时上限就是几百 KB/s。
	nettune.Conn(src)

	// 先完成握手再拨后端：握手失败就不该白拨一次内网
	if opt.TLSConfig != nil {
		// 明文 HTTP 打到这个洞上：回 302 让浏览器换 https，别一声不吭地掐掉。
		// 地址栏敲「域名:端口」时浏览器默认按 http 发，这是最常撞上的一种访问失败。
		peeked, isTLS, err := plainhttp.Sniff(src, tlsHandshakeTimeout)
		if err != nil {
			return
		}
		if !isTLS {
			plainhttp.Redirect(peeked, tlsHandshakeTimeout)
			return
		}
		src = peeked

		tlsConn := tls.Server(src, opt.TLSConfig)
		if err := tlsConn.SetDeadline(time.Now().Add(tlsHandshakeTimeout)); err != nil {
			return
		}
		if err := tlsConn.Handshake(); err != nil {
			logrus.Debugf("TLS 握手失败 [%s -> %s]: %v", src.RemoteAddr(), targetAddr, err)
			return
		}
		if err := tlsConn.SetDeadline(time.Time{}); err != nil {
			return
		}
		// tls.Conn 实现了 net.Conn，下面的转发逻辑完全不用变
		src = tlsConn
	}

	var dst net.Conn
	var err error
	if opt.BackendHTTPS {
		// 内网服务通常是自签证书，本来就无从校验
		dst, err = tls.DialWithDialer(
			&net.Dialer{Timeout: 3 * time.Second},
			protocol,
			targetAddr,
			&tls.Config{InsecureSkipVerify: true},
		)
	} else {
		dst, err = net.DialTimeout(protocol, targetAddr, 3*time.Second)
	}
	if err != nil {
		logrus.Errorf("连接内网目标失败 [%s]: %s", targetAddr, describeDialError(err))
		return
	}
	// 通往内网服务这条腿同样要调：下载是「内网读 → 外网写」，
	// 内网读到多快取决于它自己的接收窗口。（BackendHTTPS 时这里是 *tls.Conn，
	// 断言会自动跳过，那种情况由 ulimit 的系统默认兜底。）
	nettune.Conn(dst)
	defer dst.Close()

	// 流量统计：这条连接转了多字节。
	//
	// 方向定义与反向代理一致：
	//   - 入站 = 外部 → 内网（写进 dst 的）
	//   - 出站 = 内网 → 外部（写回 src 的）
	// 两边统一，仪表盘上"入站/出站"才对得上。
	inCount := &countingWriter{w: dst}
	outCount := &countingWriter{w: src}

	go func() {
		_, _ = io.Copy(inCount, src)
		dst.Close()
	}()

	_, _ = io.Copy(outCount, dst)
	src.Close()

	// 连接结束才上报：转发是长连接，中途报要额外加锁还拿不到最终值。
	// 这里的代价是"正在传的大文件要等传完才计入"，对统计趋势没有影响。
	traffic.RecordStun(deviceID, serviceID, inCount.total(), outCount.total())
}

// countingWriter 边转发边数字节。
//
// 不做成 net.Conn 包装是因为我们只需要 Write 这一个方向 ——
// 转发用的是 io.Copy，包一层 io.Writer 就够了，改动面最小。
type countingWriter struct {
	w     io.Writer
	bytes int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.bytes += int64(n)
	return n, err
}

func (c *countingWriter) total() int64 { return c.bytes }

// describeDialError 把拨号错误翻译成能直接照着改配置的话。
//
// 勾了「转发给内网时也用 HTTPS」但服务其实是明文，是这里最常见的误配置，
// 而 Go 原样抛出的 "tls: first record does not look like a TLS handshake"
// 完全没提该去动哪个开关。
func describeDialError(err error) string {
	var rec tls.RecordHeaderError
	if errors.As(err, &rec) {
		return "内网服务不是 HTTPS，却按 HTTPS 去连了——请取消该服务的「转发给内网时也用 HTTPS」勾选（原始错误：" + err.Error() + "）"
	}
	return err.Error()
}

const udpSessionTimeout = 30 * time.Second

var udpSessions sync.Map // remoteAddr.String() → net.Conn

// ForwardUDP 维护 session 表，同一客户端复用同一内部连接，持续转发回包。
func ForwardUDP(localConn *net.UDPConn, remoteAddr *net.UDPAddr, data []byte, targetAddr string, deviceID, serviceID uint) {
	key := remoteAddr.String()

	// UDP 只统计入站（收到多少字节）。回包走另一条路径且通常是广播式，
	// 逐个包记账成本高而价值低——UDP 场景看"有没有流量、多少"就够了。
	traffic.RecordStun(deviceID, serviceID, int64(len(data)), 0)

	if v, ok := udpSessions.Load(key); ok {
		v.(net.Conn).Write(data)
		return
	}

	dst, err := net.Dial("udp", targetAddr)
	if err != nil {
		logrus.Errorf("连接内网目标失败 [%s]: %v", targetAddr, err)
		return
	}

	if actual, loaded := udpSessions.LoadOrStore(key, dst); loaded {
		dst.Close()
		actual.(net.Conn).Write(data)
		return
	}

	dst.Write(data)

	go func() {
		buf := make([]byte, 65535)
		for {
			dst.SetReadDeadline(time.Now().Add(udpSessionTimeout))
			n, err := dst.Read(buf)
			if err != nil {
				break
			}
			localConn.WriteToUDP(buf[:n], remoteAddr)
		}
		udpSessions.Delete(key)
		dst.Close()
	}()
}
