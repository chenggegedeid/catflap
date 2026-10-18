// Package nettune 给 TCP 转发类场景补齐缓冲区设置。
//
// # 为什么要有这个包
//
// 反代、端口转发、内网穿透这类功能的本质都一样：把一条连接粘到另一条上，
// 中间用 io.Copy 搬字节。搬得快不快，取决于**同一时刻能在链路上放多少
// 未确认的字节**，而这个量的上限就是 TCP 收发缓冲区：
//
//	TCP 吞吐上限 ≈ min(发送缓冲, 接收缓冲) / RTT
//
// 这个式子解释了「远程下载只有几百 KB/s」这种现象的全部变量：
//   - RTT 变大：手机网络、跨运营商、走 IPv6 公网，RTT 常常在 50–150ms。
//     同一个服务在内网访问（RTT < 1ms）从来不会慢。
//   - 缓冲区停在初始值：以前全项目 SetReadBuffer / SetWriteBuffer /
//     SetNoDelay 零命中，socket 一律用系统默认。按 64KB、RTT 100ms 算，
//     吞吐上限正好 ~640KB/s —— 和实测到的 500KB/s 是同一个量级。
//
// 结论：**不是带宽小了，是窗口撑不住这条链路的延迟**。
// 把缓冲提到 1MB，同样 100ms 的 RTT 上限就到 ~10MB/s。
//
// 放在 utils 下而不是某个模块内部，是因为反代（proxy）和穿透（stun）都需要，
// 而这两个模块互不依赖 —— 各自抄一份迟早会不一样。
package nettune

import "net"

const (
	// ReadBuffer / WriteBuffer 单连接的目标缓冲大小。
	//
	// 1MB 是「够用又不浪费」：百兆~千兆家宽在跨网 RTT 下能跑起来，
	// 每连接约 2MB 内核内存也不算过分（并发几百条才到 GB 级）。
	// 现代内核开了自动调优，这里给的是初始下限，内核仍会按需上调。
	ReadBuffer  = 1 << 20
	WriteBuffer = 1 << 20
)

// Conn 给一条 TCP 连接加上能跑满高延迟链路的缓冲。
//
// 三件事，各有用处：
//   - SetReadBuffer：接收窗口，上面公式里的分子。
//   - SetWriteBuffer：发送窗口。下载/转发方向直接吃它。
//   - SetNoDelay：关 Nagle。Nagle 为了把多个小包合成一个大包会**主动延迟**
//     最多 200ms 再发；对 HTTP 这种请求-响应交织的流量，这个等待会被放大成
//     肉眼可见的卡顿。转发场景不需要为了省包而合并。
//
// 不是 TCP（Unix socket、测试里的管道）就跳过；设置失败也忽略 ——
// 后果只是退回系统默认速度，不该让连接失败。
func Conn(c net.Conn) {
	tc, ok := c.(*net.TCPConn)
	if !ok {
		return
	}
	_ = tc.SetReadBuffer(ReadBuffer)
	_ = tc.SetWriteBuffer(WriteBuffer)
	_ = tc.SetNoDelay(true)
}
