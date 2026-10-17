package home

import "testing"

/**
 * 移动分组时顺序的处理：要排到**目标组的末尾**。
 *
 * 不这么做的话，应用会带着旧组里的 order 插进新组中间 ——
 * 用户拖到"常用"，结果它出现在常用组的第二个，很莫名其妙。
 */
func TestMoveAppToCategory(t *testing.T) {
	apps := []App{
		{ID: "a1", CategoryID: "A", Order: 1},
		{ID: "a2", CategoryID: "A", Order: 2},
		{ID: "b1", CategoryID: "B", Order: 3},
		{ID: "b2", CategoryID: "B", Order: 4},
	}
	out := MoveAppToCategory(apps, "a1", "B")

	for _, a := range out {
		switch a.ID {
		case "a1":
			if a.CategoryID != "B" {
				t.Errorf("a1 应当被移到 B，实际 %s", a.CategoryID)
			}
			// B 组原来的最大 order 是 4，所以 a1 应当是 5（排在这一组末尾）
			if a.Order != 5 {
				t.Errorf("a1 应当排到 B 组末尾（order 5），实际 %d", a.Order)
			}
		case "b1":
			if a.Order != 3 {
				t.Errorf("b1 的 order 不该被改，实际 %d", a.Order)
			}
		case "b2":
			if a.Order != 4 {
				t.Errorf("b2 的 order 不该被改，实际 %d", a.Order)
			}
		}
	}
}

// 目标组是空的：从 1 开始
func TestMoveAppToEmptyCategory(t *testing.T) {
	out := MoveAppToCategory([]App{{ID: "a1", CategoryID: "A", Order: 7}}, "a1", "Empty")
	if len(out) != 1 {
		t.Fatal("长度不该变")
	}
	if out[0].CategoryID != "Empty" {
		t.Errorf("分组没改成：%s", out[0].CategoryID)
	}
	if out[0].Order != 1 {
		t.Errorf("空组应当从 1 开始，实际 %d", out[0].Order)
	}
}

// 应用不存在 → 原样返回，不臆造
func TestMoveAppNotFound(t *testing.T) {
	apps := []App{{ID: "a1", CategoryID: "A", Order: 1}}
	out := MoveAppToCategory(apps, "nope", "B")
	if len(out) != 1 || out[0].CategoryID != "A" || out[0].Order != 1 {
		t.Error("应用不存在时不该改动任何东西")
	}
}

// "目标分组存不存在"不归这个函数管 —— 它只看 Apps，分不清"空组"和"没这个组"。
// 这条测试记下这个边界：空目标组是**允许**的（排到 1），存在性校验在接口层。
func TestMoveAppExistenceCheckBelongsToCaller(t *testing.T) {
	apps := []App{{ID: "a1", CategoryID: "A", Order: 9}}
	out := MoveAppToCategory(apps, "a1", "EmptyGroup")
	if out[0].CategoryID != "EmptyGroup" || out[0].Order != 1 {
		t.Error("空目标组应当正常处理（这是接口层已确认存在的组）")
	}
}

// 已经在目标组 → 不动
func TestMoveAppSameCategory(t *testing.T) {
	apps := []App{{ID: "a1", CategoryID: "A", Order: 2}}
	out := MoveAppToCategory(apps, "a1", "A")
	if out[0].Order != 2 {
		t.Errorf("同组移动不该改 order，实际 %d", out[0].Order)
	}
}
