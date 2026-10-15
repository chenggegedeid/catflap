package stun

import (
	"testing"

	"github.com/chenggegedeid/catflap/modules/stun/model"
)

// 自动挑出来的端口必须落在该落的区间里、并且真的能绑定。
//
// 这两条是这个功能存在的理由：钉住本机端口 → NAT 给固定外部端口 →
// 用户配的「域名+端口」不会失效。
func TestPickPunchLocalPortInRangeAndFree(t *testing.T) {
	port := PickPunchLocalPort(nil, "tcp")
	if port == 0 {
		t.Fatal("没能挑出端口")
	}
	if port < punchPortMin || port > punchPortMax {
		t.Fatalf("挑到的端口 %d 不在 [%d,%d] 区间内", port, punchPortMin, punchPortMax)
	}
	if !portFree(port, "tcp") {
		t.Fatalf("挑到的端口 %d 其实绑不上", port)
	}
}

// 已经被其它服务占掉的端口要避开，否则两条服务会抢同一个本机端口。
func TestPickPunchLocalPortAvoidsUsed(t *testing.T) {
	// 把整个区间都标成已占用，此时应该挑不出来（返回 0），而不是硬挑一个撞上去
	used := make(map[uint16]bool)
	for p := uint16(punchPortMin); p <= uint16(punchPortMax); p++ {
		used[p] = true
	}
	if got := PickPunchLocalPort(used, "tcp"); got != 0 {
		t.Fatalf("全部已占用时应返回 0，got %d", got)
	}
}

// UsedPunchLocalPorts 要把各设备各服务已占的端口收集全。
func TestUsedPunchLocalPorts(t *testing.T) {
	devices := []model.Device{
		{
			DeviceID: 1,
			Services: []model.Service{
				{ID: 1, PunchLocalPort: 30001},
				{ID: 2, PunchLocalPort: 0}, // 没钉过的不能算进去
			},
		},
		{
			DeviceID: 2,
			Services: []model.Service{{ID: 3, PunchLocalPort: 30002}},
		},
	}

	used := UsedPunchLocalPorts(devices)
	if len(used) != 2 {
		t.Fatalf("应收集到 2 个端口，got %d", len(used))
	}
	if !used[30001] || !used[30002] {
		t.Fatal("已配置的端口没被收集到")
	}
	if used[0] {
		t.Fatal("端口 0（未配置）不该被当成已占用")
	}
}
