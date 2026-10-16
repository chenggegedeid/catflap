package proxy

import (
	"os"
	"strings"
	"testing"

	"github.com/chenggegedeid/catflap/modules/proxy/model"
)

// setupTempConfigDir SyncIPs 系列会落盘到 config/proxyConfig.json ——
// 必须切到临时目录，否则会在源码树里留下 modules/proxy/config/（误提交过两次）。
func setupTempConfigDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	if err := os.MkdirAll("config", 0o755); err != nil {
		t.Fatal(err)
	}
}

// TestSyncIPsAfterDeviceChange 改设备 IP 后，站点/转发里按旧 IP 写的
// backend/target 必须被替换成新 IP，且**端口要保留**。
// 这是"改设备 IP 不丢归属"功能的核心 —— 今天出过清空站点的事故之后，
// 这条测试就是回归防线。
func TestSyncIPsAfterDeviceChange(t *testing.T) {
	setupTempConfigDir(t)
	Runtime.Config = model.ProxyConfig{
		Sites: []model.Site{
			{ID: 1, Hosts: []string{"a.test"}, Backend: "192.168.9.5:7863"},
			{ID: 2, Hosts: []string{"b.test"}, Backend: "192.168.9.5"},    // 老数据可能没端口
			{ID: 3, Hosts: []string{"c.test"}, Backend: "192.168.9.6:80"}, // 别的设备，不该动
		},
		Forwards: []model.Forward{
			{ID: 1, ListenPort: 6000, Target: "192.168.9.5:22"},
			{ID: 2, ListenPort: 6001, Target: "10.0.0.1:80"}, // 别的网段，不该动
		},
	}

	if err := SyncIPsAfterDeviceChange("192.168.9.5", "192.168.9.77"); err != nil {
		t.Fatalf("同步失败: %v", err)
	}

	wantBackends := map[uint]string{1: "192.168.9.77:7863", 2: "192.168.9.77", 3: "192.168.9.6:80"}
	for _, s := range Runtime.Config.Sites {
		if s.Backend != wantBackends[s.ID] {
			t.Errorf("站点 #%d backend = %q, want %q", s.ID, s.Backend, wantBackends[s.ID])
		}
	}
	wantTargets := map[uint]string{1: "192.168.9.77:22", 2: "10.0.0.1:80"}
	for _, f := range Runtime.Config.Forwards {
		if f.Target != wantTargets[f.ID] {
			t.Errorf("转发 #%d target = %q, want %q", f.ID, f.Target, wantTargets[f.ID])
		}
	}
}

// TestSyncIPsNoPortColon 回归：backend 原来没有端口时，替换后**不能**多出一个
// 空冒号（"192.168.9.77:" —— 后端解析不出端口，转发直接失败）。用户实测踩过。
func TestSyncIPsNoPortColon(t *testing.T) {
	setupTempConfigDir(t)
	Runtime.Config = model.ProxyConfig{
		Sites: []model.Site{{ID: 1, Backend: "192.168.9.1"}},
	}
	if err := SyncIPsAfterDeviceChange("192.168.9.1", "192.168.9.2"); err != nil {
		t.Fatalf("同步失败: %v", err)
	}
	if strings.HasSuffix(Runtime.Config.Sites[0].Backend, ":") {
		t.Errorf("backend 不该以空冒号结尾: %q", Runtime.Config.Sites[0].Backend)
	}
}

// TestSyncIPsNoop 相同 IP / 空 IP：什么都不做（幂等）。
func TestSyncIPsNoop(t *testing.T) {
	setupTempConfigDir(t)
	Runtime.Config = model.ProxyConfig{
		Sites: []model.Site{{ID: 1, Backend: "192.168.9.1:80"}},
	}
	if err := SyncIPsAfterDeviceChange("192.168.9.1", "192.168.9.1"); err != nil {
		t.Fatalf("相同 IP 不该报错: %v", err)
	}
	if err := SyncIPsAfterDeviceChange("", "192.168.9.2"); err != nil {
		t.Fatalf("空 IP 不该报错: %v", err)
	}
	if Runtime.Config.Sites[0].Backend != "192.168.9.1:80" {
		t.Errorf("noop 情况下 backend 不该变: %q", Runtime.Config.Sites[0].Backend)
	}
}

// TestDeviceIDByIP 按 backend 的 IP 反查设备 ID —— 保存站点/转发时给
// deviceId 兜底（前端没传时）。查不到返回 0，绝不能编一个 ID 出来。
func TestDeviceIDByIP(t *testing.T) {
	cases := []struct {
		addr string
		want uint
	}{
		{"192.168.9.5:7863", 0}, // 没有设备表时返回 0（不 panic 不乱填）
		{"192.168.9.5", 0},
		{"", 0},
	}
	for _, c := range cases {
		if got := deviceIDByIP(c.addr); got != c.want {
			t.Errorf("deviceIDByIP(%q) = %d, want %d", c.addr, got, c.want)
		}
	}
}

// TestSortForwards 转发按 (SortOrder, ID) 排 —— 视图不排的话，
// 拖完排序一刷新就回原样，表现为「拖动没反应」。
func TestSortForwards(t *testing.T) {
	in := []model.Forward{
		{ID: 3, SortOrder: 1},
		{ID: 1}, // 没排过，SortOrder=0，应排最前
		{ID: 2, SortOrder: 2},
	}
	out := SortForwards(in)
	got := []uint{out[0].ID, out[1].ID, out[2].ID}
	want := []uint{1, 3, 2}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("排序结果 %v, want %v", got, want)
		}
	}
	// 不能改动入参
	if in[0].ID != 3 {
		t.Error("SortForwards 不该改动入参切片")
	}
}
