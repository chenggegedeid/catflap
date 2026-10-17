package home

import "testing"

func mkApps(specs ...[3]interface{}) []App {
	out := make([]App, 0, len(specs))
	for _, s := range specs {
		out = append(out, App{
			ID:         s[0].(string),
			CategoryID: s[1].(string),
			Order:      s[2].(int),
		})
	}
	return out
}

func orderOf(apps []App, id string) int {
	for _, a := range apps {
		if a.ID == id {
			return a.Order
		}
	}
	return -1
}

// 组内换位：只改这两张的 order，别的组不受影响
func TestReorderWithinGroups(t *testing.T) {
	apps := mkApps(
		[3]interface{}{"a1", "A", 1},
		[3]interface{}{"a2", "A", 2},
		[3]interface{}{"a3", "A", 3},
		[3]interface{}{"b1", "B", 4},
		[3]interface{}{"b2", "B", 5},
	)
	// 把 a3 拖到第一位
	out := ReorderWithinGroups(apps, []string{"a3", "a1", "a2"})

	if got := orderOf(out, "a3"); got != 1 {
		t.Errorf("a3 应当拿到最小的槽位 1，实际 %d", got)
	}
	if got := orderOf(out, "a1"); got != 2 {
		t.Errorf("a1 应当是 2，实际 %d", got)
	}
	if got := orderOf(out, "a2"); got != 3 {
		t.Errorf("a2 应当是 3，实际 %d", got)
	}

	// **关键**：B 组一点都不能动
	if got := orderOf(out, "b1"); got != 4 {
		t.Errorf("B 组的 b1 不该被改，实际 %d", got)
	}
	if got := orderOf(out, "b2"); got != 5 {
		t.Errorf("B 组的 b2 不该被改，实际 %d", got)
	}
}

// 只拖一部分：没点名的成员按原顺序接在后面
func TestReorderWithinGroupsKeepsUnlisted(t *testing.T) {
	apps := mkApps(
		[3]interface{}{"a1", "A", 1},
		[3]interface{}{"a2", "A", 2},
		[3]interface{}{"a3", "A", 3},
	)
	out := ReorderWithinGroups(apps, []string{"a3"})

	if got := orderOf(out, "a3"); got != 1 {
		t.Errorf("a3 应当排到最前，实际 %d", got)
	}
	if got := orderOf(out, "a1"); got != 2 {
		t.Errorf("a1 应当顺延到 2，实际 %d", got)
	}
	if got := orderOf(out, "a2"); got != 3 {
		t.Errorf("a2 应当顺延到 3，实际 %d", got)
	}
}

// 空输入原样返回
func TestReorderWithinGroupsEmpty(t *testing.T) {
	apps := mkApps([3]interface{}{"a1", "A", 1})
	out := ReorderWithinGroups(apps, nil)
	if len(out) != 1 || out[0].Order != 1 {
		t.Error("空输入不该改动任何东西")
	}
}

// order 有空洞时也要正常工作（不要求连续）
func TestReorderWithinGroupsHandlesGaps(t *testing.T) {
	apps := mkApps(
		[3]interface{}{"a1", "A", 10},
		[3]interface{}{"a2", "A", 30},
	)
	out := ReorderWithinGroups(apps, []string{"a2", "a1"})
	// 槽位集合 {10,30} 重新分配：a2 拿 10，a1 拿 30
	if got := orderOf(out, "a2"); got != 10 {
		t.Errorf("a2 应当是 10，实际 %d", got)
	}
	if got := orderOf(out, "a1"); got != 30 {
		t.Errorf("a1 应当是 30，实际 %d", got)
	}
}
