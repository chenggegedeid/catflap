package stun

import (
	"math/rand"
	"net"
	"strconv"
	"strings"

	"github.com/chenggegedeid/catflap/modules/stun/model"
)

// 自动分配打洞本机端口的范围。
//
// 取这段而不从 1024 开始：低位端口常有别的程序占着，也容易撞上常见服务的
// 默认端口；取 IANA 动态端口区间（49152-65535）里偏低的一段，既避开常见服务，
// 又留给系统临时端口足够的空间。
const (
	punchPortMin = 30000
	punchPortMax = 60000
	// 最多试这么多次。这个区间有 3 万个端口，碰壁 60 次都挑不出来基本说明
	// 机器端口快用完了，此时退回打洞时随机，不卡住建服务。
	punchPortTries = 60
)

// PickPunchLocalPort 给新建的穿透服务挑一个本机端口。
//
// 为什么要在建服务时就钉一个：NAT 的外部端口是照着「本机 IP + 本机端口」发的，
// 本机端口每次随机，外部端口就每次都变，用户配好的「域名+端口」很快失效。
//
// 为什么不让用户填：他不知道哪个端口空着，填了容易撞上已占用的；而自动挑
// 可以真的去试一下端口能不能绑定。挑中之后写进配置，这个服务之后一直用它，
// 于是外部端口稳定；服务删掉了配置里就没这条了，端口自然回到可分配状态。
//
// used 是其它服务已经占用的端口，挑的时候要避开。
// protocol 是传输协议（TCP/UDP），只检查对应协议的端口能不能绑。
//
// 返回 0 表示实在挑不出来，此时打洞会退回随机（服务照样能建、能跑，
// 只是外部端口不再稳定）。
func PickPunchLocalPort(used map[uint16]bool, protocol string) uint16 {
	for i := 0; i < punchPortTries; i++ {
		port := uint16(punchPortMin + rand.Intn(punchPortMax-punchPortMin+1))
		if used[port] {
			continue
		}
		if !portFree(port, protocol) {
			continue
		}
		return port
	}
	return 0
}

// portFree 这个本机端口现在能不能绑上。
//
// 真的去 Listen 一下再关掉，而不是查 /proc/net/tcp：后者要在几种格式之间
// 猜，跨平台还不一样，而「试绑」的结果就是我们要的那个答案。
// 关掉之后端口立刻可用（没设 SO_LINGER，不会进 TIME_WAIT 的麻烦）。
func portFree(port uint16, protocol string) bool {
	addr := ":" + strconv.Itoa(int(port))

	if strings.EqualFold(protocol, "udp") {
		conn, err := net.ListenPacket("udp", addr)
		if err != nil {
			return false
		}
		_ = conn.Close()
		return true
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

// UsedPunchLocalPorts 收集配置里各服务已经占掉的打洞本机端口。
// 新建服务时用它避开，免得两条服务钉到同一个端口上互相打架。
func UsedPunchLocalPorts(devices []model.Device) map[uint16]bool {
	used := make(map[uint16]bool)
	for _, d := range devices {
		for _, s := range d.Services {
			if s.PunchLocalPort > 0 {
				used[s.PunchLocalPort] = true
			}
		}
	}
	return used
}
