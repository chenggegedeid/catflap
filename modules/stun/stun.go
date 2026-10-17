package stun

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/chenggegedeid/catflap/modules/cert"

	"github.com/libp2p/go-reuseport"
	"github.com/pion/stun"
	"github.com/sirupsen/logrus"
)

// STUN 内网穿透实现
func (STUNRunner) Run(ctx context.Context, req STUNRequest, onState func(STUNState)) error {

	// 验证环境，确保不缺东西
	protocol, err := formatProtocol(req.Protocol)
	if err != nil {
		return err
	}
	localIP := Runtime.Network.LocalIP
	if localIP == "" {
		return fmt.Errorf("get local IP failed")
	}
	stunServer, err := Runtime.STUNService.GetBestSTUNServer()
	if err != nil {
		stunServer, err = Runtime.STUNService.GetBackupSTUNServer()
		if err != nil {
			return err
		}
	}

	// 端口复用连接STUN服务器，钉在出口网卡上：开着 TUN 时不钉，包会被 TUN 接走。
	//
	// 本机端口是重点：NAT 的外部端口是照着「本机 IP + 本机端口」发的，本机端口
	// 每次随机，外部端口就每次都不一样，用户拿到的地址过一会儿就失效。
	// 所以优先用调用方指定的端口把它固定住；指定端口用不了（多半是被别的程序
	// 占了）就退回随机，并在日志里讲清楚——服务照样能起来，只是外部端口不再稳定。
	localAddrs := []string{fmt.Sprintf("%s:0", localIP)}
	if req.PreferredLocalPort > 0 {
		localAddrs = append(
			[]string{fmt.Sprintf("%s:%d", localIP, req.PreferredLocalPort)},
			localAddrs...,
		)
	}

	var stunConn net.Conn
	var dialErr error
	for _, la := range localAddrs {
		stunConn, dialErr = CurrentOutboundIface().DialReusable(protocol, la, stunServer)
		if dialErr == nil {
			break
		}
		if la != localAddrs[len(localAddrs)-1] {
			logrus.Warnf("指定的打洞本机端口 %d 绑定失败（%v），改用随机端口——外部端口将不再固定",
				req.PreferredLocalPort, dialErr)
		}
	}
	if stunConn == nil {
		// 可能 best STUN 掉线了，换备用的再走一遍同样的本机端口候选
		primarySTUNServer := stunServer
		stunServer, err = Runtime.STUNService.GetBackupSTUNServer()
		if err != nil {
			return fmt.Errorf("connect stun server %s failed: %v; get backup stun server: %w", primarySTUNServer, dialErr, err)
		}
		for _, la := range localAddrs {
			stunConn, dialErr = CurrentOutboundIface().DialReusable(protocol, la, stunServer)
			if dialErr == nil {
				break
			}
		}
		if stunConn == nil {
			return fmt.Errorf("connect stun server %s failed: %v; connect backup stun server %s: %w",
				primarySTUNServer, dialErr, stunServer, dialErr)
		}
	}
	defer stunConn.Close()

	var localPort uint16
	switch protocol {
	case "tcp":
		localPort = uint16(stunConn.LocalAddr().(*net.TCPAddr).Port)
	case "udp":
		localPort = uint16(stunConn.LocalAddr().(*net.UDPAddr).Port)
	}

	// 之后报出去的每个状态都带上本机端口。调度器据此记住它，下次重打时优先复用
	// 同一个端口——这样即使没配固定端口，运行期间外部端口也能保持稳定。
	// 只在函数内替换闭包，不改调用方，也不影响状态机本身。
	{
		report := onState
		onState = func(s STUNState) {
			s.LocalPort = localPort
			report(s)
		}
	}

	// 发送STUN 请求,获取外部端口
	publicIP, publicPort, err := DoStunHandshake(stunConn)
	if err != nil {
		return fmt.Errorf("stun handshake: %w", err)
	}
	if protocol == "udp" {
		logrus.Infof("UDP:%s:%d", publicIP, publicPort)
	}
	onState(STUNState{State: STUNLog, Log: fmt.Sprintf("通过STUN获取外部信息：%s %s:%d", protocol, publicIP, publicPort)})

	// 在本地端口复用监听，接受穿透后的入站连接
	var tcpListener net.Listener
	var udpListener *net.UDPConn
	listenAddr := fmt.Sprintf("%s:%d", localIP, localPort)
	if protocol == "tcp" {

		tcpListener, err = reuseport.Listen(protocol, listenAddr)
		if err != nil {
			return fmt.Errorf("listen on %s failed: %w", listenAddr, err)
		}
		defer tcpListener.Close()
	} else {
		// UDP 没有 Listener 概念，用 ListenUDP 接收入站包
		var udpListenAddr *net.UDPAddr
		udpListenAddr, resolveErr := net.ResolveUDPAddr("udp", listenAddr)
		if resolveErr != nil {
			return fmt.Errorf("resolve udp addr %s failed: %w", listenAddr, resolveErr)
		}
		// ↓ 换成 reuseport.ListenPacket，和 stunConn 共用同一个端口
		packetConn, listenErr := reuseport.ListenPacket("udp", udpListenAddr.String())
		if listenErr != nil {
			return fmt.Errorf("listen udp on %s failed: %w", listenAddr, listenErr)
		}
		udpListener = packetConn.(*net.UDPConn)
		defer udpListener.Close()

	}

	// UPNP启动 映射 网关的localPort->localip:localPort UDP 不需要upnp
	//
	// upnpActive 记录 UPnP 这条腿到底站没站住，后面保活只在这个为真时才去
	// 检测/续约映射。用 req.UseUPnP 是不行的：那是「用户想不想用」，
	// 而不是「现在能不能用」。
	upnpActive := false
	if protocol == "tcp" {
		if req.UseUPnP {
			upnpCtx, upnpCancel := context.WithTimeout(ctx, 20*time.Second)
			err := AddPortMappingByQueueWithLocalIP(
				upnpCtx,
				localPort,
				localPort,
				strings.ToUpper(protocol),
				fmt.Sprintf("EasyLink-%s", req.ServiceName),
				localIP,
			)
			upnpCancel()
			if err != nil {
				// UPnP 是锦上添花，不是打洞的前提。以前这里直接 return，
				// 结果是路由器没开 UPnP、或处于 CGNAT（路由器 WAN 是私网地址）
				// 时，打洞被 UPnP 的失败一起拖死，界面只剩"重启 4 次"。
				//
				// 实际上这类网络里 UPnP 建的映射压根不会生效，而打洞本身是
				// 能通的（同类产品不用 UPnP 照样稳定）。所以这里降级：
				// 记清楚原因，继续走纯打洞。
				logrus.Warnf("[stun] %s UPnP 端口映射失败（%v），转为纯打洞继续", req.ServiceName, err)
				onState(STUNState{State: STUNLog, Log: fmt.Sprintf("UPnP 不可用（%v），转为纯打洞", err)})
			} else {
				upnpActive = true
				defer func() {
					go DeletePortMapping(localPort, strings.ToUpper(protocol))
				}()
				onState(STUNState{State: STUNLog, Log: fmt.Sprintf("UPNP启动成功：路由:%d->%s:%d", localPort, localIP, localPort)})
			}
		} else {
			onState(STUNState{State: STUNLog, Log: "未启用 UPnP，纯打洞"})
		}
	}

	// 把端口传出去
	if onState != nil {
		onState(STUNState{State: STUNMapped, ExternalIP: publicIP, ExternalPort: uint16(publicPort)}) //todo统一一下命名
	}

	// 保活
	innerCtx, innerCancel := context.WithCancel(ctx)
	errCh := make(chan error, 2)
	defer innerCancel()
	if protocol == "tcp" { //tcp 保活
		go func() {
			if err := tcpStunHealthKeepAlive(
				innerCtx,
				stunConn,
				publicIP,
				publicPort,
				localPort,
				localIP,
				// 传「UPnP 现在到底能不能用」，不是「用户想不想用」。
				// 初始建映射失败时这里是 false，保活就不会再去碰那条不存在的
				// 映射——否则重建失败又会把已经通了的洞给掐掉。
				upnpActive,
				protocol,
				req.ServiceName,
				onState,
				func(ip string, port int) {
					if onState != nil {
						onState(STUNState{State: STUNAlive, ExternalIP: ip, ExternalPort: uint16(port)})
					}
				},
			); err != nil && ctx.Err() == nil {
				errCh <- fmt.Errorf("tcp keepalive failed: %w", err)
			}
		}()
	} else if protocol == "udp" { //udp 保活
		go func() {
			if err := udpStunHealthKeepAlive(
				innerCtx,
				stunConn,
				// udpListener, // 用来收包
				// publicIP,
				publicPort,
				localPort,
				localIP,
				func() {
					if onState != nil {
						onState(STUNState{State: STUNAlive, ExternalIP: publicIP, ExternalPort: uint16(publicPort)})
					}
				},
			); err != nil && ctx.Err() == nil {
				errCh <- fmt.Errorf("udp keepalive failed: %w", err)
			}
		}()
	}

	// 接收外网连接，转发到对应内网服务
	go func() {
		targetAddr := fmt.Sprintf("%s:%d", req.TargetIP, req.InternalPort) // 构建目标ip+端口
		if protocol == "tcp" {
			// 转发选项只构造一次，被这个洞上所有连接共享。
			// tls.Config 里的 GetCertificate 每次握手实时回调，
			// 因此证书续期后新连接自动用上新证书，不需要重启洞。
			opt := ForwardOptions{BackendHTTPS: req.BackendHTTPS}
			if req.TLSTerminate {
				opt.TLSConfig = cert.Runtime.Manager.ServerTLSConfig(req.CertID)
			}

			for {
				clientConn, err := tcpListener.Accept()
				if err != nil {
					if ctx.Err() != nil {
						errCh <- nil
					} else {
						errCh <- fmt.Errorf("listener closed unexpectedly: %w", err)
					}
					return
				}

				// 门禁：先看这个来源能不能连。穿透直接暴露在公网上，
				// 和反代站点一样需要 IP 门禁（用户在服务上配的黑/白名单，
				// 加上审计里封的 IP —— 后者是全局的，这里也一并查）。
				// 被挡下就不建转发，连接直接关掉。
				if !tunnelAccessAllowed(req, clientConn.RemoteAddr()) {
					_ = clientConn.Close()
					continue
				}

				go ForwardTCP(clientConn, targetAddr, protocol, opt, req.DeviceID, req.ServiceID) //转发到对应的内网服务
			}
		} else {
			buf := make([]byte, 65535)
			for {
				n, remoteAddr, err := udpListener.ReadFromUDP(buf)
				if err != nil {
					if ctx.Err() != nil {
						errCh <- nil
					} else {
						errCh <- fmt.Errorf("udp listener closed: %w", err)
					}
					return
				}
				data := make([]byte, n)
				copy(data, buf[:n])

				// 门禁（UDP 也要查）：UDP 无连接，按每个数据报的源地址判，
				// 不放行就直接丢掉，不建转发。
				if !tunnelAccessAllowed(req, remoteAddr) {
					continue
				}

				go ForwardUDP(udpListener, remoteAddr, data, targetAddr, req.DeviceID, req.ServiceID)
			}
		}

	}()

	// 取消时保活 goroutine 什么都不发，Accept 又要等这里返回、defer 关掉监听器才醒，
	// 只等 errCh 会两边互相等，调度器白等 12 秒才放弃
	select {
	case <-ctx.Done():
		return nil
	case err := <-errCh:
		return err
	}
}

// ===================== 内部 函数 ===============

// 格式化 Protocol 输入
func formatProtocol(protocol string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(protocol)) // 去除空格转小写
	if value == "" {
		value = "tcp"
	}
	switch value {
	case "tcp", "udp":
		return value, nil
	default:
		return "", fmt.Errorf("unsupported protocol: %s", protocol)
	}

}

// 发送 STUN 请求
func DoStunHandshake(conn net.Conn) (string, int, error) {
	msg := stun.MustBuild(stun.TransactionID, stun.BindingRequest)
	if _, err := conn.Write(msg.Raw); err != nil {
		return "", 0, fmt.Errorf("send stun request: %w", err)
	}

	conn.SetDeadline(time.Now().Add(3 * time.Second))
	defer conn.SetDeadline(time.Time{})

	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil {
		return "", 0, fmt.Errorf("read stun response: %w", err)
	}

	var response stun.Message
	response.Raw = buf[:n]
	if err = response.Decode(); err != nil {
		return "", 0, fmt.Errorf("decode stun response: %w", err)
	}

	var xorAddr stun.XORMappedAddress
	if err = xorAddr.GetFrom(&response); err != nil {
		return "", 0, fmt.Errorf("get mapped address: %w", err)
	}

	return xorAddr.IP.String(), xorAddr.Port, nil
}

// ================= TCP 保活 ========================

// STUN TCP 保活
//
// onAlive 在确认映射可用时回调，参数是当前生效的公网 IP/端口——
// CGNAT 下重连后端口可能变化，回调方（scheduler）会据此重新同步 DDNS/重定向。
func tcpStunHealthKeepAlive(
	ctx context.Context,
	currentStunConn net.Conn,
	publicIP string,
	publicPort int,
	localPort uint16,
	localIP string,
	useUPnP bool,
	protocol string,
	serviceName string,
	onState func(STUNState),
	onAlive func(publicIP string, publicPort int),
) error {

	currentIP := publicIP
	currentPort := publicPort

	/*
	   打洞用的那台 STUN 服务器要**记住**，重连时优先回到同一台。

	   原因（这是 TCP 打洞端口漂移的根因）：
	   NAT3 / 端口受限锥形 NAT 下，同一个本机端口访问**不同**的服务器 IP，
	   NAT 会分配**不同**的公网端口。而 GetBackupSTUNServer() 是"哪个先响应就用哪个"，
	   每次调用可能返回不同的服务器 —— 于是每重连一次，公网端口就换一个，
	   表现就是"端口几分钟变一次"（保活周期 25 秒）。

	   lucky/natmap 能稳定数天，就是因为它始终对着同一台服务器续期。
	*/
	var pinnedServer string
	if currentStunConn != nil {
		if ra := currentStunConn.RemoteAddr(); ra != nil {
			pinnedServer = ra.String()
		}
	}

	// 候选服务器：原来那台优先，其次才考虑换。换服务器必然导致端口变化，
	// 所以只在原服务器真的连不上时才走到后面。
	serverCandidates := func() []string {
		list := make([]string, 0, 3)
		if pinnedServer != "" {
			list = append(list, pinnedServer)
		}
		if best, err := Runtime.STUNService.GetBestSTUNServer(); err == nil {
			list = append(list, best)
		}
		if backup, err := Runtime.STUNService.GetBackupSTUNServer(); err == nil {
			list = append(list, backup)
		}
		return list
	}

	// 重连并重新握手。CGNAT 等多层 NAT 下重连后映射端口可能变化：
	// 变化后的新映射同样可以把入站流量送到本服务（同一个本地端口），
	// 所以这里不再把「端口变了」当致命错误，而是跟随新端口继续保活，
	// 并通过 onState 通知上层重新同步 DDNS / 入口重定向。
	redial := func() (net.Conn, string, int, error) {
		localAddr := fmt.Sprintf("%s:%d", localIP, localPort)
		var lastErr error
		for _, stunServer := range serverCandidates() {
			conn, dialErr := CurrentOutboundIface().DialReusable("tcp", localAddr, stunServer)
			if dialErr != nil {
				lastErr = fmt.Errorf("reconnect stun server %s failed: %w", stunServer, dialErr)
				continue
			}
			ip, port, hsErr := DoStunHandshake(conn)
			if hsErr != nil {
				conn.Close()
				lastErr = fmt.Errorf("stun handshake after reconnect failed: %w", hsErr)
				continue
			}
			// 成功了就把这台钉住，后续保活都回这里，端口才稳得住
			pinnedServer = stunServer
			return conn, ip, port, nil
		}
		return nil, "", 0, lastErr
	}

	followOrRestart := func(verifyErr error) error {
		newConn, newIP, newPort, err := redial()
		if err != nil {
			return fmt.Errorf("tcp keepalive failed: %v; %v", verifyErr, err)
		}
		currentStunConn.Close()
		remapped := newPort != currentPort || newIP != currentIP
		currentStunConn = newConn
		currentIP, currentPort = newIP, newPort
		if remapped && onAlive != nil {
			logrus.Infof("[%s] 公网映射变化 -> %s:%d，跟随新端口继续保活", serviceName, newIP, newPort)
			// 先报 MAPPED 让上层把入口重定向标脏，再报 ALIVE 触发 DDNS/重定向同步
			if onState != nil {
				onState(STUNState{State: STUNMapped, ExternalIP: newIP, ExternalPort: uint16(newPort)})
			}
			onAlive(newIP, newPort)
		} else if onAlive != nil {
			onAlive(currentIP, currentPort)
		}
		return nil
	}

	// 首次保活验证：CGNAT 等多层 NAT 下，内网自连公网地址（回环）不通属正常，
	// 不能作为健康依据。改用当前 STUN 连接重新握手，既保活又确认映射。
	//
	// 同样是"只有连不上才重连"：端口对不上只是映射变了，跟着新端口走即可，
	// 重连反而会因为换服务器（NAT3）再换一次端口。
	if ip, port, err := DoStunHandshake(currentStunConn); err != nil {
		if err := followOrRestart(err); err != nil {
			return err
		}
	} else {
		if port != currentPort || ip != currentIP {
			currentIP, currentPort = ip, port
			if onState != nil {
				onState(STUNState{State: STUNMapped, ExternalIP: ip, ExternalPort: uint16(port)})
			}
		}
		if onAlive != nil {
			onAlive(currentIP, currentPort)
		}
	}

	// 开启28s 的检查
	healthTicker := time.NewTicker(25 * time.Second)
	defer healthTicker.Stop()

	// UPnP 1min 检测 + 3h 续约 (useUPnP=false 时 channel 为 nil, select 永远不选中)
	var upnpCheckCh, upnpRenewCh <-chan time.Time
	upperProto := strings.ToUpper(protocol)
	upnpDesc := fmt.Sprintf("EasyLink-%s", serviceName)
	if useUPnP {
		upnpCheckTicker := time.NewTicker(1 * time.Minute)
		defer upnpCheckTicker.Stop()
		upnpCheckCh = upnpCheckTicker.C

		upnpRenewTicker := time.NewTicker(3 * time.Hour)
		defer upnpRenewTicker.Stop()
		upnpRenewCh = upnpRenewTicker.C
	}

	for {
		select {
		case <-ctx.Done():
			return nil

		case <-upnpCheckCh:
			checkCtx, checkCancel := context.WithTimeout(ctx, 10*time.Second)
			exists, err := CheckPortMappingByQueue(checkCtx, localPort, upperProto)
			checkCancel()
			if err != nil {
				logrus.Warnf("UPnP 端口映射检测失败 %d: %v", localPort, err)
				continue
			}
			if exists {
				continue
			}
			logrus.Warnf("UPnP 端口映射丢失，尝试重建: %d", localPort)
			rebuildCtx, rebuildCancel := context.WithTimeout(ctx, 20*time.Second)
			err = AddPortMappingByQueueWithLocalIP(rebuildCtx, localPort, localPort, upperProto, upnpDesc, localIP)
			rebuildCancel()
			if err != nil {
				return fmt.Errorf("UPnP 端口映射重建失败: %w", err)
			}
			logrus.Infof("UPnP 端口映射重建成功: %d", localPort)

		case <-upnpRenewCh:
			renewCtx, renewCancel := context.WithTimeout(ctx, 20*time.Second)
			err := AddPortMappingByQueueWithLocalIP(renewCtx, localPort, localPort, upperProto, upnpDesc, localIP)
			renewCancel()
			if err != nil {
				return fmt.Errorf("UPnP 端口映射续约失败: %w", err)
			}
			logrus.Infof("UPnP 端口映射续约成功: %d", localPort)

		case <-healthTicker.C:

			/*
			   保活：用**现有连接**重新握手一次就够了。

			   关键点是区分两种情况，以前没区分，导致端口一直漂：
			     - 连接还在、只是公网端口变了 → 更新记录即可，**不要重连**。
			       重连会换服务器（NAT3 下换服务器 = 换端口），于是"变了又变"，
			       形成"端口变 → 重连 → 端口再变"的循环，这正是漂移的放大器。
			     - 连接断了（握手报错）→ 才重连，且固定回原来那台服务器。

			   握手本身产生的流量就是保活心跳，不需要额外发包。
			*/
			ip, port, err := DoStunHandshake(currentStunConn)
			if err != nil {
				// 连接不可用：重连（候选列表里原来那台服务器排在最前）
				if err := followOrRestart(err); err != nil {
					return err
				}
				continue
			}

			if port != currentPort || ip != currentIP {
				logrus.Infof("[%s] 公网映射变化 -> %s:%d，跟随新端口继续保活（不重连）",
					serviceName, ip, port)
				currentIP, currentPort = ip, port
				// 先报 MAPPED 让上层把入口重定向标脏，再触发 DDNS/重定向同步
				if onState != nil {
					onState(STUNState{State: STUNMapped, ExternalIP: ip, ExternalPort: uint16(port)})
				}
			}

			if onAlive != nil {
				onAlive(currentIP, currentPort)
			}
		}
	}

}

// 类似 Time.Sleep 但是开被ctx 终终端
func sleepWithCtx(ctx context.Context, d time.Duration) bool {
	select {
	case <-time.After(d):
		return true
	case <-ctx.Done():
		return false
	}
}

// TCP STUN 梯度健康检查
// 注意：依赖 NAT 回环（从内网自连公网地址），CGNAT 等多层 NAT 环境下恒失败，
// 主流程已改用 tcpStunVerifyMapping（同连接 STUN 握手）替代，此函数保留备用。
func tcpStunHealthCheck(ctx context.Context, publicIP string, expectedPublicPort int) bool {
	// 第一次检查
	if tcpConnectCheck(publicIP, expectedPublicPort, 3*time.Second) {
		return true
	}

	// 梯度检查1 3 5
	sleepTime := 1 * time.Second
	for i := 0; i < 3; i++ {
		if !sleepWithCtx(ctx, sleepTime) {
			return false
		}
		if tcpConnectCheck(publicIP, expectedPublicPort, 3*time.Second) {
			return true
		}
		sleepTime += 2 * time.Second
	}
	return false
}

// tcpStunVerifyMapping 在同一条 STUN 连接上重新握手，验证 NAT 映射端口未变，顺便产生保活流量。
// CGNAT 等不支持 NAT 回环的环境下，这比从内网自连公网地址更可靠。
func tcpStunVerifyMapping(stunConn net.Conn, expectedPublicPort int) error {
	_, port, err := DoStunHandshake(stunConn)
	if err != nil {
		return err
	}
	if port != expectedPublicPort {
		return fmt.Errorf("public port changed from %d to %d", expectedPublicPort, port)
	}
	return nil
}

// TCP检查是否存活
func tcpConnectCheck(host string, port int, timeout time.Duration) bool {
	// JoinHostPort 而不是 "%s:%d"：IPv6 字面量必须带方括号
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// ================= UDP 保活 ========================

// UDP STUN 保活
// UDP 没有连接性，状态，目前只能用检测端口变没变来保活，是不是真的打通了得看过滤行为了
func udpStunHealthKeepAlive(
	ctx context.Context,
	currentStunConn net.Conn,
	publicPort int,
	localPort uint16,
	localIP string,
	onAlive func(),
) error {
	// 刚打通视为活
	if onAlive != nil {
		onAlive()
	}

	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:

			// 正常检查：发 binding request 顺便保活，同时确认端口
			newPort, err := udpStunHealthCheck(ctx, currentStunConn)
			if err == nil {
				if newPort != publicPort {
					return fmt.Errorf("public port changed %d -> %d", publicPort, newPort)
				}
				continue
			}
			if ctx.Err() != nil {
				return nil
			}

			// 3次连接失败，尝试重连
			currentStunConn.Close()
			newConn, newPort, err := udpReconnectSTUN(localIP, localPort)
			if err != nil {
				return fmt.Errorf("stun reconnect: %w", err)
			}
			if newPort != publicPort {
				newConn.Close()
				return fmt.Errorf("public port changed %d -> %d after reconnect", publicPort, newPort)
			}
			currentStunConn = newConn
		}
	}
}

// 梯度重试 1s 2s 3s，容忍网络抖动
func udpStunHealthCheck(ctx context.Context, conn net.Conn) (int, error) {
	delay := time.Second
	for range 3 {
		if !sleepWithCtx(ctx, delay) {
			return 0, ctx.Err()
		}
		_, port, err := DoStunHandshake(conn)
		if err == nil {
			return port, nil
		}
		delay += 1 * time.Second
	}
	return 0, fmt.Errorf("UDP stun retry exhausted")
}

// 重连 STUN 服务器并握手
func udpReconnectSTUN(localIP string, localPort uint16) (net.Conn, int, error) {
	stunServer, err := Runtime.STUNService.GetBestSTUNServer()
	if err != nil {
		stunServer, err = Runtime.STUNService.GetBackupSTUNServer()
		if err != nil {
			return nil, 0, fmt.Errorf("get stun server: %w", err)
		}
	}
	conn, err := CurrentOutboundIface().DialReusable("udp", fmt.Sprintf("%s:%d", localIP, localPort), stunServer)
	if err != nil {
		return nil, 0, fmt.Errorf("dial stun %s: %w", stunServer, err)
	}
	_, port, err := DoStunHandshake(conn)
	if err != nil {
		conn.Close()
		return nil, 0, fmt.Errorf("handshake: %w", err)
	}
	return conn, port, nil
}

// STUN 梯度检测
// func udpStunHealthCheck(ctx context.Context, publicIP string, publicPort int, localPort uint16) bool {
// 	if udpConnectCheck(publicIP, publicPort, localPort, 3*time.Second) {
// 		return true
// 	}
// 	sleepTime := 1 * time.Second
// 	for i := 0; i < 3; i++ {
// 		if !sleepWithCtx(ctx, sleepTime) {
// 			return false
// 		}
// 		if udpConnectCheck(publicIP, publicPort, localPort, 3*time.Second) {
// 			return true
// 		}
// 		sleepTime += 2 * time.Second
// 	}
// 	return false
// }

// // UDP STUN 保活
// func udpStunHealthKeepAlive(
// 	ctx context.Context,
// 	currentStunConn net.Conn,
// 	// udpListener *net.UDPConn,
// 	publicIP string,
// 	publicPort int,
// 	localPort uint16,
// 	localIP string,
// 	onAlive func(),
// ) error {
// 	// 先做一次外部连通性验证
// 	if !udpStunHealthCheck(ctx, publicIP, publicPort, localPort) {
// 		if ctx.Err() != nil {
// 			return nil
// 		}
// 		return fmt.Errorf("udp health check failed")
// 	}
// 	if onAlive != nil {
// 		onAlive()
// 	}

// 	// UDP NAT 映射超时通常 30s，用 20s 心跳保住映射
// 	keepAliveTicker := time.NewTicker(20 * time.Second)
// 	// 外部连通性检查周期可以长一些
// 	healthTicker := time.NewTicker(28 * time.Second)
// 	defer keepAliveTicker.Stop()
// 	defer healthTicker.Stop()

// 	for {
// 		select {
// 		case <-ctx.Done():
// 			return nil

// 		case <-keepAliveTicker.C:
// 			// 向 STUN 服务器发心跳，维持 NAT 映射
// 			msg := stun.MustBuild(stun.TransactionID, stun.BindingRequest)
// 			currentStunConn.SetWriteDeadline(time.Now().Add(3 * time.Second))
// 			currentStunConn.Write(msg.Raw)
// 			currentStunConn.SetWriteDeadline(time.Time{})

// 		case <-healthTicker.C:
// 			if udpStunHealthCheck(ctx, publicIP, publicPort, localPort) {
// 				continue
// 			}

// 			// 外部连通性失败，先用现有 conn 重新握手确认端口
// 			_, port, err := DoStunHandshake(currentStunConn)
// 			if err == nil {
// 				if port != publicPort {
// 					return fmt.Errorf("public port changed from %d to %d", publicPort, port)
// 				}
// 				continue
// 			}

// 			// stunConn 断了，重新建连
// 			currentStunConn.Close()
// 			stunServer, err := Runtime.STUNService.GetBestSTUNServer()
// 			if err != nil {
// 				stunServer, err = Runtime.STUNService.GetBackupSTUNServer()
// 				if err != nil {
// 					return fmt.Errorf("get stun server for reconnect: %w", err)
// 				}
// 			}

// 			localAddr := fmt.Sprintf("%s:%d", localIP, localPort)
// 			newConn, dialErr := reuseport.Dial("udp", localAddr, stunServer)
// 			if dialErr != nil {
// 				return fmt.Errorf("reconnect stun server %s failed: %w", stunServer, dialErr)
// 			}

// 			_, newPort, handshakeErr := DoStunHandshake(newConn)
// 			if handshakeErr != nil {
// 				newConn.Close()
// 				return fmt.Errorf("stun handshake after reconnect failed: %w", handshakeErr)
// 			}

// 			if newPort != publicPort {
// 				newConn.Close()
// 				return fmt.Errorf("public port changed from %d to %d", publicPort, newPort)
// 			}

// 			currentStunConn = newConn
// 		}
// 	}
// }

// // UDP 梯度健康检查
// func udpStunHealthCheck(ctx context.Context, publicIP string, publicPort int, localPort uint16) bool {
// 	if udpConnectCheck(publicIP, publicPort, localPort, 3*time.Second) {
// 		return true
// 	}
// 	sleepTime := 1 * time.Second
// 	for i := 0; i < 3; i++ {
// 		if !sleepWithCtx(ctx, sleepTime) {
// 			return false
// 		}
// 		if udpConnectCheck(publicIP, publicPort, localPort, 3*time.Second) {
// 			return true
// 		}
// 		sleepTime += 2 * time.Second
// 	}
// 	return false
// }

// // 向 publicIP:publicPort 发一个探测包，用临时 socket 收回包，不干扰 udpListener
// func udpConnectCheck(publicIP string, publicPort int, localPort uint16, timeout time.Duration) bool {
// 	// 发探测包
// 	dst := fmt.Sprintf("%s:%d", publicIP, publicPort)
// 	probe, err := net.Dial("udp", dst)
// 	if err != nil {
// 		return false
// 	}
// 	defer probe.Close()

// 	probe.SetDeadline(time.Now().Add(timeout))
// 	if _, err = probe.Write([]byte("ping")); err != nil {
// 		return false
// 	}

// 	// 单独开一个临时 socket 在同端口收回包，不碰 udpListener
// 	tempConn, err := reuseport.ListenPacket("udp", fmt.Sprintf(":%d", localPort))
// 	if err != nil {
// 		return false
// 	}
// 	defer tempConn.Close()

// 	tempConn.SetDeadline(time.Now().Add(timeout))
// 	buf := make([]byte, 64)
// 	_, _, err = tempConn.ReadFrom(buf)
// 	return err == nil
// }
