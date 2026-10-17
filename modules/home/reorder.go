package home

import "sort"

/**
 * 按分组本地重排。
 *
 * 输入是被拖动的应用 id 顺序（按新位置排好），输出是改好的 Apps 切片。
 *
 * **为什么不能全局重编号 1..N**：order 是全局唯一的，但界面上一屏只显示一个分组。
 * 全局重编号意味着"在 A 组拖动"会改掉 B 组应用的 order —— 实际表现就是
 * 拖完 A 组，B 组的某个应用被挤到了最前面。这个 bug 真的发生过。
 *
 * 正确做法：只动被点名应用**所在分组**的 order 槽位。
 * 每个分组自己维护一组升序的 order 值，组内换位置时只是把这些值重新分配，
 * 不碰别的分组。
 */
func ReorderWithinGroups(apps []App, ids []string) []App {
	if len(ids) == 0 {
		return apps
	}
	index := make(map[string]int, len(ids))
	for i, id := range ids {
		index[id] = i
	}

	// 找出受影响的分组（去重）
	affected := map[string]bool{}
	for _, a := range apps {
		if _, ok := index[a.ID]; ok {
			affected[a.CategoryID] = true
		}
	}

	out := append([]App(nil), apps...)

	for catID := range affected {
		// 取出这一组的成员索引，连同它们占用的 order 槽位
		members := make([]int, 0, len(out))
		slots := make([]int, 0, len(out))
		for i := range out {
			if out[i].CategoryID == catID {
				members = append(members, i)
				slots = append(slots, out[i].Order)
			}
		}
		sort.Ints(slots)

		// 组内排序：被点名的按新位置在前，没点名的按原 order 接在后面
		sort.SliceStable(members, func(i, j int) bool {
			ai, aj := out[members[i]].ID, out[members[j]].ID
			pi, iok := index[ai]
			pj, jok := index[aj]
			if iok && jok {
				return pi < pj
			}
			if iok != jok {
				return iok
			}
			return out[members[i]].Order < out[members[j]].Order
		})

		// 把这批槽位按升序重新分配回去
		for k, mi := range members {
			if k < len(slots) {
				out[mi].Order = slots[k]
			}
		}
	}

	return out
}
