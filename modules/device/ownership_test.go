package device

import (
	"testing"
)

// TestGetByIP 按 IP 找设备：归属逻辑的兜底路径（老数据按 IP 关联）。
// 找不到时必须返回 nil —— 调用方靠它判断"这条规则未归到设备"，
// 绝不能返回一个零值设备让规则被错误归属。
func TestGetByIP(t *testing.T) {
	d, err := Add("测试机A", "10.99.0.1")
	if err != nil {
		t.Fatalf("加设备失败: %v", err)
	}
	defer func() { _ = Delete(d.DeviceID) }()

	got := GetByIP("10.99.0.1")
	if got == nil {
		t.Fatal("已存在的 IP 应该找得到")
	}
	if got.DeviceID != d.DeviceID {
		t.Errorf("ID 不匹配: want %d got %d", d.DeviceID, got.DeviceID)
	}
	if got.Name != "测试机A" {
		t.Errorf("名字不匹配: got %q", got.Name)
	}

	if got := GetByIP("10.99.0.99"); got != nil {
		t.Errorf("不存在的 IP 应返回 nil，却返回了 %+v", got)
	}
	if got := GetByIP(""); got != nil {
		t.Errorf("空 IP 应返回 nil，却返回了 %+v", got)
	}
}

// TestUpdateIPSyncs 前置：改设备 IP 时，通过 SetIPChangedHook 注册的
// 同步回调必须被调用（proxy/stun 里缓存的 IP 字符串靠它更新）。
// 这里只验证 hook 机制本身 —— proxy 侧的替换逻辑在 proxy 包测。
func TestUpdateIPCallsHook(t *testing.T) {
	d, err := Add("测试机B", "10.99.0.2")
	if err != nil {
		t.Fatalf("加设备失败: %v", err)
	}
	defer func() { _ = Delete(d.DeviceID) }()

	var gotOld, gotNew string
	var called bool
	prev := onIPChanged
	SetIPChangedHook(func(oldIP, newIP string) error {
		called = true
		gotOld, gotNew = oldIP, newIP
		return nil
	})
	defer SetIPChangedHook(prev) // 测完恢复，避免污染其它测试

	if _, err := Update(d.DeviceID, "测试机B", "10.99.0.3"); err != nil {
		t.Fatalf("改 IP 失败: %v", err)
	}
	if !called {
		t.Fatal("改 IP 后同步回调没有被调用")
	}
	if gotOld != "10.99.0.2" || gotNew != "10.99.0.3" {
		t.Errorf("回调参数不对: old=%q new=%q", gotOld, gotNew)
	}
}

// TestUpdateIPConflict 改 IP 撞上另一台已占用的 IP：必须拒绝，
// 且错误要能被 errors.Is(err, ErrIPExists) 识别（api 层靠它给 409 + 提示）。
func TestUpdateIPConflict(t *testing.T) {
	a, err := Add("甲", "10.99.1.1")
	if err != nil {
		t.Fatalf("加设备失败: %v", err)
	}
	defer func() { _ = Delete(a.DeviceID) }()
	b, err := Add("乙", "10.99.1.2")
	if err != nil {
		t.Fatalf("加设备失败: %v", err)
	}
	defer func() { _ = Delete(b.DeviceID) }()

	if _, err := Update(b.DeviceID, "乙", "10.99.1.1"); err == nil {
		t.Fatal("改成别人占用的 IP 应该被拒绝")
	}
	// 乙的 IP 不能被改动
	after, _ := Get(b.DeviceID)
	if after.IP != "10.99.1.2" {
		t.Errorf("被拒绝后 IP 不应变: got %q", after.IP)
	}
}

// TestEnsureLocalhostOnlyLanIP 本机自动入库只加"最像 LAN"的那一个 IP。
// 这台测试机上可能有多块网卡/网桥（docker 的 172.x），全加进去会把设备页刷屏
// —— 实测踩过：一次启动多出 3 条 docker 网桥设备。
func TestEnsureLocalhostOnlyLanIP(t *testing.T) {
	before := len(List())
	EnsureLocalhost()
	after := len(List())
	if after > before+1 {
		t.Errorf("EnsureLocalhost 最多加 1 台，却从 %d 变成 %d", before, after)
	}
	ips := localIPs()
	t.Logf("本机候选 IP（按 LAN 相似度排序）: %v", ips)
}

// TestLanScore 网卡排序：192.168 > 10 > 172 > 其它。
// EnsureLocalhost 取第一个 —— 排错的话会把 docker 网桥当成"本机"加进设备表。
func TestLanScore(t *testing.T) {
	cases := []struct {
		ip   string
		want int
	}{
		{"192.168.9.8", 4},
		{"10.0.0.5", 3},
		{"172.17.0.1", 2},
		{"8.8.8.8", 1},
	}
	for _, c := range cases {
		if got := lanScore(c.ip); got != c.want {
			t.Errorf("lanScore(%q) = %d, want %d", c.ip, got, c.want)
		}
	}
}
