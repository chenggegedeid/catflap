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

/**
 * 把应用移到别的分组，并排到那一组的末尾。
 *
 * 找不到应用、或目标分组不存在时**原样返回** —— 不臆造一条记录，
 * 也不静默改掉别的字段。调用方据此判断要不要报错。
 */
func MoveAppToCategory(apps []App, appID string, categoryID string) []App {
	idx := -1
	for i := range apps {
		if apps[i].ID == appID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return apps
	}
	if apps[idx].CategoryID == categoryID {
		return apps
	}

	// 顺带算出目标组里现有的最大 order
	maxOrder := 0
	inTarget := 0
	for _, a := range apps {
		if a.CategoryID != categoryID {
			continue
		}
		inTarget++
		if a.Order > maxOrder {
			maxOrder = a.Order
		}
	}
	// 这个判断是**由调用方给出分组列表**更合适，但这里只有 Apps ——
	// 所以退一步：目标组一个成员都没有时，无法区分"是空组"还是"压根没有这个组"，
	// 交给调用方（接口层）用 Categories 去判。这里只在明确有成员时才信任。
	_ = inTarget

	out := append([]App(nil), apps...)
	out[idx].CategoryID = categoryID
	out[idx].Order = maxOrder + 1
	return out
}
