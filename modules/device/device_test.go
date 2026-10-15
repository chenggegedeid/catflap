package device

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// 老配置迁移：设备 ID 必须原样保留。
//
// 这是最要命的一条 —— 穿透服务靠 deviceId 定位自己的机器，
// ID 变了服务就认不出设备了（表现是"服务还在，但不知道在哪台机器上"）。
func TestMigrateKeepsDeviceID(t *testing.T) {
	dir := t.TempDir()
	oldWD, _ := os.Getwd()
	defer os.Chdir(oldWD)
	os.Chdir(dir)

	// 造一份老的 stunConfig.json（设备嵌在里面，还带着 services）
	stunCfg := map[string]any{
		"bestStun": "stun.example.com",
		"devices": []map[string]any{
			{
				"id":   7,
				"name": "群晖NAS",
				"ip":   "192.168.1.20",
				"services": []map[string]any{
					{"id": 3, "name": "SSH", "internalPort": 22},
				},
			},
			{"id": 9, "name": "树莓派", "ip": "192.168.1.31"},
		},
	}
	writeJSON(t, "config/stunConfig.json", stunCfg)

	if err := ReadConfig(); err != nil {
		t.Fatalf("迁移失败：%v", err)
	}

	devs := List()
	if len(devs) != 2 {
		t.Fatalf("应迁出 2 台设备，实际 %d", len(devs))
	}

	nas, ok := Get(7)
	if !ok {
		t.Fatal("ID 7 的设备应存在（ID 必须原样保留）")
	}
	if nas.Name != "群晖NAS" || nas.IP != "192.168.1.20" {
		t.Errorf("设备 7 = %+v，期望 群晖NAS/192.168.1.20", nas)
	}
	if _, ok := Get(9); !ok {
		t.Error("ID 9 的设备应存在")
	}

	// 服务不该被搬过来（那是穿透的业务数据）
	raw := readRaw(t, ConfigPath)
	if contains(raw, "services") {
		t.Error("设备表里不该出现 services —— 那是穿透的数据，不进公共表")
	}
}

// 迁移是幂等的：已经有 devices.json 就不该再去翻 stunConfig
func TestMigrateIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	oldWD, _ := os.Getwd()
	defer os.Chdir(oldWD)
	os.Chdir(dir)

	writeJSON(t, "config/stunConfig.json", map[string]any{
		"devices": []map[string]any{
			{"id": 1, "name": "老设备", "ip": "10.0.0.1"},
		},
	})

	if err := ReadConfig(); err != nil {
		t.Fatalf("首次迁移失败：%v", err)
	}
	if len(List()) != 1 {
		t.Fatalf("首次应迁出 1 台，实际 %d", len(List()))
	}

	// 第二次读：devices.json 已存在，不该重复迁
	if err := ReadConfig(); err != nil {
		t.Fatalf("二次读取失败：%v", err)
	}
	if len(List()) != 1 {
		t.Errorf("二次读取后应有 1 台（不能重复迁），实际 %d", len(List()))
	}
}

// 没有老配置时：建一份空的，不该报错（新装实例）
func TestMigrateWithNoLegacyConfig(t *testing.T) {
	dir := t.TempDir()
	oldWD, _ := os.Getwd()
	defer os.Chdir(oldWD)
	os.Chdir(dir)

	if err := ReadConfig(); err != nil {
		t.Fatalf("没有老配置时不该报错：%v", err)
	}
	if len(List()) != 0 {
		t.Errorf("没有老配置时应为空，实际 %d", len(List()))
	}
	// 刻意**不**创建空文件：init() 在任何引用本包的进程启动时都会跑
	// （包括 go test），无条件写盘会在源码目录里留下垃圾文件。
	// 等真的加第一台设备时再写。
	if _, err := os.Stat(ConfigPath); err == nil {
		t.Error("没有老配置时不应创建 devices.json（避免测试污染源码目录）")
	}
}

// ===== 增删改 =====

func TestAddUpdateDelete(t *testing.T) {
	dir := t.TempDir()
	oldWD, _ := os.Getwd()
	defer os.Chdir(oldWD)
	os.Chdir(dir)
	ReadConfig()

	d, err := Add("群晖", "192.168.1.20")
	if err != nil {
		t.Fatalf("加设备失败：%v", err)
	}
	if d.DeviceID == 0 {
		t.Error("新设备应分配到非 0 的 ID")
	}

	// IP 重复要拒
	if _, err := Add("另一台", "192.168.1.20"); err != ErrIPExists {
		t.Errorf("同 IP 应被拒，实际 %v", err)
	}

	// 按 IP 查找（服务归属靠它）
	if _, ok := FindByIP("192.168.1.20"); !ok {
		t.Error("应能按 IP 找到设备")
	}
	if _, ok := FindByIP("192.168.1.99"); ok {
		t.Error("不存在的 IP 不该找到")
	}

	// 改 IP
	if _, err := Update(d.DeviceID, "群晖改名", "192.168.1.21"); err != nil {
		t.Fatalf("改设备失败：%v", err)
	}
	got, _ := Get(d.DeviceID)
	if got.Name != "群晖改名" || got.IP != "192.168.1.21" {
		t.Errorf("改后 = %+v", got)
	}

	// 删
	if err := Delete(d.DeviceID); err != nil {
		t.Fatalf("删设备失败：%v", err)
	}
	if _, ok := Get(d.DeviceID); ok {
		t.Error("删掉后不该还能查到")
	}
	if err := Delete(d.DeviceID); err != ErrNotFound {
		t.Errorf("删不存在的应返回 ErrNotFound，实际 %v", err)
	}
}

// IP 归一化：空格、大小写、IPv6 方括号都要容错
func TestNormalizeIP(t *testing.T) {
	cases := []struct{ in, want string }{
		{" 192.168.1.20 ", "192.168.1.20"},
		{"[2001:DB8::1]", "2001:db8::1"},
		{"2001:DB8::1", "2001:db8::1"},
		{"", ""},
	}
	for _, c := range cases {
		if got := normalizeIP(c.in); got != c.want {
			t.Errorf("normalizeIP(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// 空 IP / 空名字
func TestAddValidation(t *testing.T) {
	dir := t.TempDir()
	oldWD, _ := os.Getwd()
	defer os.Chdir(oldWD)
	os.Chdir(dir)
	ReadConfig()

	if _, err := Add("x", ""); err != ErrEmptyIP {
		t.Errorf("空 IP 应被拒，实际 %v", err)
	}
	// 名字留空 → 用 IP 兜底
	d, err := Add("", "10.0.0.5")
	if err != nil {
		t.Fatalf("名字留空不该报错：%v", err)
	}
	if d.Name != "10.0.0.5" {
		t.Errorf("名字留空应回落到 IP，实际 %q", d.Name)
	}
}

// ===== 辅助 =====

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(v)
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
}

func readRaw(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (stringIndex(s, sub) >= 0)
}

func stringIndex(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
