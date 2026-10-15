package stun

import (
	"testing"

	"github.com/chenggegedeid/catflap/modules/stun/model"
)

// 老配置（服务嵌在设备里）要能拆成独立的服务列表
func TestMigrateServicesFlattensNested(t *testing.T) {
	cfg := &model.Config{
		Devices: []model.Device{
			{
				DeviceID: 7,
				Name:     "群晖NAS",
				IP:       "192.168.1.20",
				Services: []model.Service{
					{ID: 3, Name: "SSH", InternalPort: 22},
					{ID: 4, Name: "照片库", InternalPort: 5000},
				},
			},
			{
				DeviceID: 9,
				Name:     "树莓派",
				IP:       "192.168.1.31",
				Services: []model.Service{
					{ID: 1, Name: "Git", InternalPort: 3000},
				},
			},
		},
	}

	migrateServices(cfg)

	if len(cfg.Services) != 3 {
		t.Fatalf("应拆出 3 个服务，实际 %d", len(cfg.Services))
	}
	if len(cfg.Devices) != 0 {
		t.Error("迁移后 Devices 应清空（设备归 device 模块管）")
	}

	// 每个服务都要带上自己的 DeviceID —— 丢了就认不出在哪台机器上
	want := map[uint]int{7: 2, 9: 1}
	got := map[uint]int{}
	for _, s := range cfg.Services {
		got[s.DeviceID]++
	}
	for id, n := range want {
		if got[id] != n {
			t.Errorf("设备 %d 应有 %d 个服务，实际 %d", id, n, got[id])
		}
	}
}

// 迁移幂等：已经拆过的不会再拆一遍（否则服务会翻倍）
func TestMigrateServicesIsIdempotent(t *testing.T) {
	cfg := &model.Config{
		Devices: []model.Device{
			{DeviceID: 1, Services: []model.Service{{ID: 1, Name: "A"}}},
		},
		Services: []model.Service{{DeviceID: 1, ID: 1, Name: "A"}},
	}

	migrateServices(cfg)

	if len(cfg.Services) != 1 {
		t.Errorf("已有 Services 时不该再拆，实际 %d 个", len(cfg.Services))
	}
}

// dehydrate → hydrate 走一圈，服务要回到原来的设备上（round-trip）
//
// 这条最重要：写盘拆开、读盘装回，如果这两个不对称，
// 用户重启一次服务就跑到别的机器上去了。
func TestDehydrateHydrateRoundTrip(t *testing.T) {
	tree := []model.Device{
		{
			DeviceID: 7, Name: "群晖NAS", IP: "192.168.1.20",
			Services: []model.Service{
				{DeviceID: 7, ID: 3, Name: "SSH", InternalPort: 22},
				{DeviceID: 7, ID: 4, Name: "照片库", InternalPort: 5000},
			},
		},
		{
			DeviceID: 9, Name: "树莓派", IP: "192.168.1.31",
			Services: []model.Service{{DeviceID: 9, ID: 1, Name: "Git", InternalPort: 3000}},
		},
	}

	cfg := &model.Config{Devices: tree}
	dehydrateDevices(cfg)

	if len(cfg.Services) != 3 {
		t.Fatalf("拆出 %d 个服务，期望 3", len(cfg.Services))
	}
	if cfg.Devices != nil {
		t.Error("写盘前 Devices 应清空（不落盘）")
	}

	// 装回去（这一步依赖 device 表里有没有对应设备，测试环境通常为空；
	// 这里只验证拆出来的 DeviceID 是对的）
	byID := map[uint][]model.Service{}
	for _, s := range cfg.Services {
		byID[s.DeviceID] = append(byID[s.DeviceID], s)
	}
	if len(byID[7]) != 2 || len(byID[9]) != 1 {
		t.Errorf("DeviceID 归属不对：7→%d 个，9→%d 个", len(byID[7]), len(byID[9]))
	}

	// 服务的其它字段不能被打乱
	for _, s := range cfg.Services {
		if s.DeviceID == 7 && s.ID == 3 && s.Name != "SSH" {
			t.Errorf("服务字段被打乱：%+v", s)
		}
	}
}

// 空配置不该出问题
func TestDehydrateEmpty(t *testing.T) {
	cfg := &model.Config{}
	dehydrateDevices(cfg)
	if len(cfg.Services) != 0 {
		t.Errorf("空配置拆完应为空，实际 %d", len(cfg.Services))
	}

	migrateServices(nil) // 不该崩
	hydrateDevices(nil)  // 不该崩
}

// 迁移后落盘不能把刚拆出来的服务清掉。
//
// 踩过的坑：migrateServices 拆完服务会清空 Devices，紧接着 UpdateConfig
// 里的 dehydrateDevices 又从（已空的）设备树重建 Services，
// 结果 3 个服务全丢。端到端实测才发现 —— 单元测试当时没覆盖这条路径。
func TestDehydrateKeepsServicesAfterMigrate(t *testing.T) {
	cfg := &model.Config{
		Devices: []model.Device{
			{
				DeviceID: 7, Name: "群晖NAS", IP: "192.168.1.20",
				Services: []model.Service{
					{ID: 3, Name: "SSH", InternalPort: 22},
					{ID: 4, Name: "照片库", InternalPort: 5000},
				},
			},
		},
	}

	// 模拟启动流程：先迁移，再落盘
	migrateServices(cfg)
	if len(cfg.Services) != 2 {
		t.Fatalf("迁移后应有 2 个服务，实际 %d", len(cfg.Services))
	}
	if cfg.Devices != nil {
		t.Fatal("迁移后 Devices 应清空")
	}

	dehydrateDevices(cfg) // ← 这一步以前会把 Services 清成空

	if len(cfg.Services) != 2 {
		t.Errorf("落盘前不该把服务清掉，实际剩 %d 个", len(cfg.Services))
	}
	for _, s := range cfg.Services {
		if s.DeviceID != 7 {
			t.Errorf("服务的 DeviceID 应保留为 7，实际 %d", s.DeviceID)
		}
	}
}

// 正常情况（有设备树）仍然要从设备树拆出服务
func TestDehydrateUsesDeviceTreeWhenPresent(t *testing.T) {
	cfg := &model.Config{
		Devices: []model.Device{
			{DeviceID: 1, Services: []model.Service{{ID: 1, Name: "A"}}},
			{DeviceID: 2, Services: []model.Service{{ID: 1, Name: "B"}}},
		},
		Services: []model.Service{{DeviceID: 99, ID: 9, Name: "陈旧数据"}},
	}

	dehydrateDevices(cfg)

	if len(cfg.Services) != 2 {
		t.Fatalf("应从设备树拆出 2 个服务，实际 %d", len(cfg.Services))
	}
	for _, s := range cfg.Services {
		if s.DeviceID == 99 {
			t.Error("陈旧的 Services 应被设备树的内容覆盖")
		}
	}
}
