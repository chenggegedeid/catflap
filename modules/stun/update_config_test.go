package stun

import (
	"os"
	"testing"

	"github.com/chenggegedeid/catflap/modules/stun/model"
)

// TestUpdateConfigRefreshesRuntimeServices 保存配置后，内存里的服务列表必须跟着更新。
//
// 为什么必须：Services（扁平列表）是**真相来源**，设备树是派生的
// （hydrateDevices 从 Services 组装 Devices；dehydrateDevices 反向拆回去落盘）。
//
// 以前 UpdateConfig 是「按值传参 → dehydrate 出 Services → 写盘」就结束了，
// 从不把拆出来的 Services 写回 Runtime.Config。于是**新建 / 改过的服务
// 只存在于设备树里**，Runtime.Config.Services 还是启动时那一份旧数据，
// 后果有四个，每一个都很难从界面上看出原因：
//
//  1. LookupService 查不到新服务 → 选了通知渠道也一条不发（不是配错，是查不到）
//  2. 同一个函数拿不到 Domain → 通知里的地址退回 IP（终结 TLS 的服务连不上）
//  3. tunnel_access.go 的 IP 门禁查不到服务 → 黑名单对新建服务完全不生效
//  4. 下一次 hydrateDevices（比如在设备页改了设备）会用旧表重建设备树，
//     新服务从界面上消失，再落盘一次就永久没了 —— 数据丢失
//
// 所以这条测试守的是"保存真的生效"，不是某个函数的返回值。
func TestUpdateConfigRefreshesRuntimeServices(t *testing.T) {
	// UpdateConfig 会写 config/stunConfig.json，别把测试产物落在仓库里
	t.Chdir(t.TempDir())
	if err := os.MkdirAll("config", 0o755); err != nil {
		t.Fatalf("建临时配置目录失败: %v", err)
	}

	old := Runtime.Config
	defer func() { Runtime.Config = old }()

	Runtime.Config = model.Config{
		Services: []model.Service{{DeviceID: 7, ID: 1, Name: "老服务"}},
		Devices:  []model.Device{{DeviceID: 7, Name: "dev"}},
	}
	// 模拟「新建服务」：stun_service_add.go 只往设备树里 append，
	// 这正是以前会丢的那一环
	Runtime.Config.Devices[0].Services = []model.Service{
		{DeviceID: 7, ID: 1, Name: "老服务"},
		{DeviceID: 7, ID: 2, Name: "新服务", Domain: "a.example.com"},
	}

	if err := UpdateConfig(Runtime.Config); err != nil {
		t.Fatalf("UpdateConfig 失败: %v", err)
	}

	got := LookupService(7, 2)
	if got == nil {
		t.Fatal("新建的服务在 Runtime.Config.Services 里查不到（LookupService 返回 nil）——" +
			"通知不发、门禁失效、地址退回 IP，都是这一个根因")
	}
	if got.Name != "新服务" {
		t.Errorf("服务名不对: 期望 %q，实际 %q", "新服务", got.Name)
	}
	if got.Domain != "a.example.com" {
		t.Errorf("域名没同步过来，通知里的地址会退回 IP: %q", got.Domain)
	}
	// 老的也要还在
	if LookupService(7, 1) == nil {
		t.Error("原有服务被弄丢了")
	}
}
