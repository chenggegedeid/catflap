package proxy

import (
	"net"

	"github.com/chenggegedeid/catflap/utils/nettune"
)

// ===================== TCP 吞吐调优 =====================
//
// 真正的实现在 utils/nettune —— 那里解释了完整背景：
//
//	TCP 吞吐上限 ≈ min(发送缓冲, 接收缓冲) / RTT
//
// 远程访问的 RTT（手机网络、跨运营商、公网 IPv6）常常 50–150ms，
// 而反代这条路上以前一处缓冲区都没设过（全项目 SetReadBuffer /
// SetWriteBuffer / SetNoDelay 零命中），停在系统初始值。按 64KB、
// RTT 100ms 算，上限正好 ~640KB/s —— 和实测到的 500KB/s 是同一个量级。
//
// 这不是带宽不够，是**窗口撑不住这条链路的延迟**。提到 1MB 后，
// 同样 100ms 的 RTT 上限就到 ~10MB/s。
//
// 这里只留用户友好的转发层，不复制那里的逻辑。
func tuneConn(c net.Conn) { nettune.Conn(c) }

// tunedListener 在 Accept 返回前把缓冲套上去。
//
// **必须包在最底层**（TLS / plainhttp 之前）：Serve Accept 出来的连接，
// 往上还会被 plainhttp、TLS 各包一层。若包在它们外面，拿到的是
// *tls.Conn 或包装类型，nettune 里断言不到 *net.TCPConn，就会静默地
// 什么都不做 —— 这种「改了但没生效」最难发现，所以专门做一层监听器，
// 而不是在上层某个地方记得调一下。
type tunedListener struct {
	net.Listener
}

func (l tunedListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	tuneConn(c)
	return c, nil
}
