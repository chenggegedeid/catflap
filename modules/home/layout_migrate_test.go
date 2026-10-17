package home

import (
	"testing"
	"time"
)

// 老配置：只有 pagedOrder / scrollOrder，没有 order / size
func legacyConfig() *Config {
	now := time.Now()
	return &Config{
		Title:            "Catflap",
		LegacyLayoutMode: "paged-free",
		NetworkPrefer:    "wanV4",
		Categories:       []Category{{ID: "c1", Name: "常用", Order: 1}},
		Apps: []App{
			{ID: "a", Name: "A", LegacyPagedOrder: 3, LegacyScrollOrder: 1, CategoryID: "c1"},
			{ID: "b", Name: "B", LegacyPagedOrder: 1, LegacyScrollOrder: 2, CategoryID: "c1"},
			{ID: "c", Name: "C", LegacyPagedOrder: 2, LegacyScrollOrder: 3, CategoryID: "c1"},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func TestMigrateLayoutAssignsOrderByLegacySequence(t *testing.T) {
	cfg := legacyConfig()

	if !migrateLayout(cfg) {
		t.Fatal("老配置应当触发迁移")
	}

	// 老顺序 pagedOrder: b(1) < c(2) < a(3)，迁移后 order 应当照这个相对顺序
	want := map[string]int{"b": 1, "c": 2, "a": 3}
	for _, a := range cfg.Apps {
		if a.Order != want[a.ID] {
			t.Errorf("%s 的 order = %d，期望 %d", a.ID, a.Order, want[a.ID])
		}
		if a.Size != defaultCardSize {
			t.Errorf("%s 的 size = %q，期望默认 %q", a.ID, a.Size, defaultCardSize)
		}
	}

	if cfg.SchemaVersion != LayoutSchemaV2 {
		t.Errorf("schemaVersion = %d，期望 %d", cfg.SchemaVersion, LayoutSchemaV2)
	}
	if cfg.LegacyLayoutMode != "" {
		t.Errorf("旧布局模式应当被清空，实际是 %q", cfg.LegacyLayoutMode)
	}
}

// 迁移必须是幂等的：跑第二次什么都不该变
func TestMigrateLayoutIsIdempotent(t *testing.T) {
	cfg := legacyConfig()
	migrateLayout(cfg)

	before := make([]int, len(cfg.Apps))
	for i, a := range cfg.Apps {
		before[i] = a.Order
	}

	if migrateLayout(cfg) {
		t.Error("第二次迁移不该再改动任何东西")
	}
	for i, a := range cfg.Apps {
		if a.Order != before[i] {
			t.Errorf("%s 的 order 在第二次迁移时变了：%d → %d", a.ID, before[i], a.Order)
		}
	}
}

// 全是 0 的老配置（用户从来没拖过排序）不能崩，也不能全排成同一个位置
func TestMigrateLayoutHandlesAllZeroOrders(t *testing.T) {
	cfg := &Config{
		Apps: []App{
			{ID: "a", Name: "A"},
			{ID: "b", Name: "B"},
			{ID: "c", Name: "C"},
		},
	}
	migrateLayout(cfg)

	seen := map[int]bool{}
	for _, a := range cfg.Apps {
		if a.Order <= 0 {
			t.Errorf("%s 的 order 是 %d，没排上", a.ID, a.Order)
		}
		if seen[a.Order] {
			t.Errorf("order %d 重复了（%s）—— 重复会让前端抢位置", a.Order, a.ID)
		}
		seen[a.Order] = true
	}
}

// 已经迁过但被手工改坏的（order 全 0）要能补回来
func TestFillMissingOrderRepairsBrokenConfig(t *testing.T) {
	cfg := &Config{
		SchemaVersion: LayoutSchemaV2,
		Apps: []App{
			{ID: "a", Order: 2, Size: "1x1"},
			{ID: "b", Order: 2, Size: "1x1"}, // 和 a 撞了
			{ID: "c", Order: 0, Size: ""},    // 没排
		},
	}
	if !migrateLayout(cfg) {
		t.Fatal("坏配置应当被修好")
	}
	seen := map[int]bool{}
	for _, a := range cfg.Apps {
		if a.Order <= 0 || seen[a.Order] {
			t.Errorf("%s 的 order = %d，期望 1..N 且不重复", a.ID, a.Order)
		}
		seen[a.Order] = true
		if a.Size == "" {
			t.Errorf("%s 的 size 应当被补上默认值", a.ID)
		}
	}
}

// 新配置一建出来就该是 V2，不需要再迁
func TestNewConfigIsAlreadyV2(t *testing.T) {
	cfg := &Config{SchemaVersion: LayoutSchemaV2}
	if migrateLayout(cfg) {
		t.Error("新配置不该触发迁移")
	}
}
