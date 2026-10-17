package stun

import (
	"fmt"

	"net"
	"strings"
	"sync"
	"time"

	"github.com/pion/stun"
	"github.com/sirupsen/logrus"
)

type STUNServiceDelay struct {
	Server string
	Delay  time.Duration
}

type STUNService struct {
	BestSTUNServer       string
	AvailableSTUNServers []string
	AllSTUNServers       []string

	updating sync.Mutex // 保证 Update 单次运行
}

// 创建并初始化 STUNService
func NewSTUNService(allSTUNServers []string) *STUNService {
	s := &STUNService{
		AllSTUNServers: allSTUNServers,
	}
	s.UpdateSTUNService()
	return s
}

// 返回最优STUN服务器
func (s *STUNService) GetBestSTUNServer() (string, error) {
	if s.BestSTUNServer == "" {
		return "", fmt.Errorf("no best STUN server")
	}

	return s.BestSTUNServer, nil
}

// 返回备用STUN服务器
func (s *STUNService) GetBackupSTUNServer() (string, error) {

	// 如果没有可用服务器，先更新在返回
	if len(s.AvailableSTUNServers) == 0 {
		s.UpdateSTUNService()
		if s.BestSTUNServer == "" {
			return "", fmt.Errorf("no best STUN server")
		}
		return s.BestSTUNServer, nil
	}

	var wg sync.WaitGroup
	resultCh := make(chan STUNServiceDelay, len(s.AvailableSTUNServers))

	//遍历测试返回可用的
	wg.Add(len(s.AvailableSTUNServers))
	for _, server := range s.AvailableSTUNServers {
		go func() {
			defer wg.Done()
			serverDeelay, err := getSTUNServerDelay(server)
			if err != nil {
				return
			}

			resultCh <- *serverDeelay
		}()
	}

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	/*
	   收集**全部**可用结果后取**延时最小**的那台，而不是"谁先响应用谁"。

	   原来只取第一个 `<-resultCh`，而各 goroutine 的完成顺序取决于网络抖动，
	   于是同一个函数连续两次调用可能返回不同的服务器。

	   这一点对 TCP 打洞很致命：NAT3（端口受限锥形）下，同一个本机端口访问
	   不同的服务器 IP 会被分配不同的公网端口 —— 保活时一换服务器，公网端口
	   就跟着变，表现就是"打洞端口几分钟漂移一次"。
	   选最稳定的（延时最低）那台，能让重连尽量回到同一台服务器。
	*/
	var best string
	var bestDelay time.Duration
	got := false
	for serverDelay := range resultCh {
		if !got || serverDelay.Delay < bestDelay {
			best, bestDelay, got = serverDelay.Server, serverDelay.Delay, true
		}
	}
	if got {
		return best, nil
	}

	//如果遍历完了都没有
	s.UpdateSTUNService()
	if s.BestSTUNServer == "" {
		return s.BestSTUNServer, fmt.Errorf("no available STUN server")
	}
	return s.BestSTUNServer, nil
}

// 更新当前STUN服务器
func (s *STUNService) UpdateSTUNService() {
	// 只能单次运行,当有运行，等待运行完成即可返回
	if !s.updating.TryLock() {
		s.updating.Lock()
		s.updating.Unlock()
		return
	}
	defer s.updating.Unlock()

	results := make(chan STUNServiceDelay, len(s.AllSTUNServers))

	// 遍历测试每一个STUN服务器
	var wg sync.WaitGroup
	for _, server := range s.AllSTUNServers {
		wg.Add(1)
		go func(srv string) {
			defer wg.Done()
			res, err := getSTUNServerDelay(srv)
			if err != nil {
				logrus.Info(err)
				return
			}
			logrus.Infof("STUN服务器 %s 延时: %v", res.Server, res.Delay)
			results <- *res
		}(server)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	// 获取最优的STUN服务器
	var bestServer string
	var bestDelay time.Duration = time.Hour
	var available []string
	for res := range results {
		available = append(available, res.Server)
		if res.Delay < bestDelay {
			bestDelay = res.Delay
			bestServer = res.Server
		}
	}

	s.BestSTUNServer = bestServer
	s.AvailableSTUNServers = available
}

// NormalizeSTUNServer 校验并归一化一个 STUN 服务器地址。
// 接受 "host:port" / "host"（补默认 3478），返回规范形式；
// 空串、非法字符、端口越界都算错。
func NormalizeSTUNServer(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("地址不能为空")
	}
	// 统一小写域名；可能有 scheme 前缀的顺手剥掉
	s = strings.TrimPrefix(strings.TrimPrefix(strings.ToLower(s), "stun://"), "stun:")
	host, portStr, err := net.SplitHostPort(s)
	if err != nil {
		// 没带端口，补默认 3478
		host = s
		portStr = "3478"
	}
	if host == "" {
		return "", fmt.Errorf("主机名不能为空")
	}
	// host 只允许 域名字符 / 数字 / 点 / 连字符 / IPv6 方括号
	cleanHost := strings.Trim(host, "[]")
	for _, r := range cleanHost {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == ':') {
			return "", fmt.Errorf("主机名含有非法字符 %q", string(r))
		}
	}
	var port int
	if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil || port < 1 || port > 65535 {
		return "", fmt.Errorf("端口 %q 不合法", portStr)
	}
	return net.JoinHostPort(host, portStr), nil
}

// RefreshSTUNService 用新的服务器列表替换当前列表并立即重测选优。
// 正在跑的打洞服务不受影响，新一次打洞/重连起用新列表。
func RefreshSTUNService(servers []string) {
	if Runtime.STUNService == nil {
		Runtime.STUNService = NewSTUNService(servers)
		return
	}
	Runtime.STUNService.AllSTUNServers = servers
	Runtime.STUNService.AvailableSTUNServers = nil
	Runtime.STUNService.BestSTUNServer = ""
	Runtime.STUNService.UpdateSTUNService()
}

// 获取连接延时
//
// 从出口网卡发出：开着 TUN 时不指定出口，量到的是到代理节点的延迟，
// 挑出来的「最快」服务器对真实出口毫无意义。
func getSTUNServerDelay(srv string) (*STUNServiceDelay, error) {
	star := time.Now()

	// 建立TCP连接
	conn, err := CurrentOutboundIface().DialTCP(srv, 3*time.Second)
	if err != nil {
		return nil, fmt.Errorf("TCP connection failed to be established")
	}
	defer conn.Close()

	// 发送stun请求
	msg := stun.MustBuild(stun.TransactionID, stun.BindingRequest)
	_, err = conn.Write(msg.Raw)
	if err != nil {
		return nil, fmt.Errorf("Sending STUN request failed")
	}

	// 读取响应
	conn.SetDeadline(time.Now().Add(3 * time.Second))
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if n == 0 || err != nil {
		return nil, fmt.Errorf("%s read failed:", err)
	}

	// 解析响应，拿到映射地址
	var response stun.Message
	response.Raw = buf[:n]
	if err := response.Decode(); err != nil {
		return nil, fmt.Errorf("decode STUN failed: %w", err)
	}
	var xorAddr stun.XORMappedAddress
	if err := xorAddr.GetFrom(&response); err != nil {
		return nil, fmt.Errorf("get mapped address failed: %w", err)
	}

	// 返回的不是公网IP，视为超时/不可用
	if isNonPublicIP(xorAddr.IP) {
		return nil, fmt.Errorf("STUN服务器 %s 返回非公网IP: %s", srv, xorAddr.IP.String())
	}

	dalay := time.Since(star)

	return &STUNServiceDelay{Server: srv, Delay: dalay}, nil
}

// 判断是不是不可用作公网的IP（私网/回环/链路本地/CGNAT等）
func isNonPublicIP(ip net.IP) bool {
	if ip == nil || ip.IsUnspecified() || ip.IsLoopback() ||
		ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsMulticast() {
		return true
	}
	// CGNAT 100.64.0.0/10，IsPrivate 不覆盖
	if ip4 := ip.To4(); ip4 != nil && ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127 {
		return true
	}
	return false
}
