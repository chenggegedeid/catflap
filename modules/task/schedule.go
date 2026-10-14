package task

import (
	"fmt"
	"time"
)

// NextDue 算出这条任务下一次该跑的时刻。
//
// 单独拆成函数是为了能写单测：调度逻辑的 bug 表现得很隐蔽 ——
// 「到点了却没跑」在日志里一片安静，用户只会觉得这功能时灵时不灵。
func NextDue(t Task, now time.Time) (time.Time, error) {
	switch t.Kind {
	case KindInterval:
		if t.IntervalMin <= 0 {
			return time.Time{}, fmt.Errorf("间隔必须大于 0 分钟")
		}
		return now.Add(time.Duration(t.IntervalMin) * time.Minute), nil

	case KindDaily:
		if t.Hour < 0 || t.Hour > 23 || t.Minute < 0 || t.Minute > 59 {
			return time.Time{}, fmt.Errorf("每天的时刻要在 00:00 到 23:59 之间")
		}
		// 今天那个时刻
		due := time.Date(now.Year(), now.Month(), now.Day(), t.Hour, t.Minute, 0, 0, now.Location())
		// 今天已经过了就顺延到明天 —— 启动时补跑一次昨天的是自找麻烦：
		// 半夜两点关过机的话，早上开机就会连着补一串不该现在跑的任务。
		if !due.After(now) {
			due = due.AddDate(0, 0, 1)
		}
		return due, nil

	default:
		return time.Time{}, fmt.Errorf("不认识的调度方式 %q", t.Kind)
	}
}

// Due 判断此刻该不该跑。
//
// lastRun 为零值时按「还没跑过」处理：interval 类立刻跑一次，
// daily 类则等到当天那个时刻。
func Due(t Task, now time.Time, lastRun time.Time) bool {
	if !t.Enabled {
		return false
	}
	switch t.Kind {
	case KindInterval:
		if t.IntervalMin <= 0 {
			return false
		}
		if lastRun.IsZero() {
			return true
		}
		return !now.Before(lastRun.Add(time.Duration(t.IntervalMin) * time.Minute))
	case KindDaily:
		if lastRun.IsZero() {
			// 没跑过：等到当天那个时刻（或之后）才跑
			target := time.Date(now.Year(), now.Month(), now.Day(), t.Hour, t.Minute, 0, 0, now.Location())
			return !now.Before(target)
		}
		// 跑过：今天还没跑，且已过当天时刻
		if isSameDay(lastRun, now) {
			return false
		}
		target := time.Date(now.Year(), now.Month(), now.Day(), t.Hour, t.Minute, 0, 0, now.Location())
		return !now.Before(target)
	default:
		return false
	}
}

func isSameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}
