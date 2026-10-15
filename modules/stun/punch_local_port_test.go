package stun

import (
	"testing"

	"github.com/chenggegedeid/catflap/modules/stun/model"
)

// 打洞的本机端口要能从服务配置带进请求里。
//
// 背景：NAT 的外部端口是照着「本机 IP + 本机端口」分配的，本机端口每次随机，
// 外部端口就每次都变（实测同一条服务一分钟内 3103 → 3555 → 3304 → 3220），
// 用户配好的「域名+端口」很快失效。修复就是把用户配的固定本机端口带进
// STUNRequest，让 Run 优先绑定它。
func TestBuildSTUNRequestCarriesPunchLocalPort(t *testing.T) {
	device := &model.Device{DeviceID: 1, IP: "192.168.9.5"}
	service := &model.Service{
		ID:             1,
		Name:           "nas",
		InternalPort:   2573,
		Protocol:       "TCP",
		PunchLocalPort: 35545,
	}

	req := buildSTUNRequest(device, service)
	if req.PreferredLocalPort != 35545 {
		t.Fatalf("配置的固定本机端口没带进请求：want 35545, got %d", req.PreferredLocalPort)
	}

	// 0 = 自动（老行为），必须原样透传，不能被顺手填成别的
	service.PunchLocalPort = 0
	req = buildSTUNRequest(device, service)
	if req.PreferredLocalPort != 0 {
		t.Fatalf("未配置时应保持 0（随机），got %d", req.PreferredLocalPort)
	}
}

// 调度器要记住上一次打洞用的本机端口，下一轮重打时优先复用——
// 这样即使没配固定端口，运行期间外部端口也不会一直跳。
func TestServiceEntryRemembersLocalPort(t *testing.T) {
	e := newServiceEntry(nil, 1, 1, "设备", "服务")

	if got := e.getLocalPort(); got != 0 {
		t.Fatalf("初始应为 0，got %d", got)
	}

	e.setLocalPort(35545)
	if got := e.getLocalPort(); got != 35545 {
		t.Fatalf("set 之后要能读回来：want 35545, got %d", got)
	}

	// 覆盖写：新一轮用的端口变了，要跟新
	e.setLocalPort(35546)
	if got := e.getLocalPort(); got != 35546 {
		t.Fatalf("重复写入要覆盖：want 35546, got %d", got)
	}
}
