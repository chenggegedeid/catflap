package task

import (
	"testing"
	"time"
)

func TestNextDueInterval(t *testing.T) {
	now := time.Date(2026, 10, 14, 10, 0, 0, 0, time.Local)
	got, err := NextDue(Task{Kind: KindInterval, IntervalMin: 30}, now)
	if err != nil {
		t.Fatal(err)
	}
	if want := now.Add(30 * time.Minute); !got.Equal(want) {
		t.Errorf("NextDue = %v，期望 %v", got, want)
	}
	if _, err := NextDue(Task{Kind: KindInterval, IntervalMin: 0}, now); err == nil {
		t.Error("间隔为 0 时应该报错")
	}
}

// TestNextDueDailyRollsToTomorrow 今天那个时刻已经过了，要顺延到明天。
//
// 这条守的是一个真实场景：半夜两点关过机，早上开机 —— 如果「补跑昨天那次」，
// 用户早上会看到一串本该在夜里跑的任务集中触发（比如集中唤醒一堆机器）。
func TestNextDueDailyRollsToTomorrow(t *testing.T) {
	now := time.Date(2026, 10, 14, 10, 0, 0, 0, time.Local)
	got, err := NextDue(Task{Kind: KindDaily, Hour: 2, Minute: 0}, now)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 10, 15, 2, 0, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Errorf("NextDue = %v，期望 %v", got, want)
	}
}

func TestNextDueDailySameDayWhenNotYetPassed(t *testing.T) {
	now := time.Date(2026, 10, 14, 10, 0, 0, 0, time.Local)
	got, err := NextDue(Task{Kind: KindDaily, Hour: 18, Minute: 30}, now)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 10, 14, 18, 30, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Errorf("NextDue = %v，期望 %v", got, want)
	}
}

func TestDueInterval(t *testing.T) {
	now := time.Date(2026, 10, 14, 10, 0, 0, 0, time.Local)
	task := Task{Enabled: true, Kind: KindInterval, IntervalMin: 60}

	// 从来没跑过 → 立刻跑
	if !Due(task, now, time.Time{}) {
		t.Error("没跑过的间隔任务应该立刻跑一次")
	}
	// 59 分钟前跑过 → 还不到
	if Due(task, now, now.Add(-59*time.Minute)) {
		t.Error("还没到间隔就不该跑")
	}
	// 61 分钟前跑过 → 到点了
	if !Due(task, now, now.Add(-61*time.Minute)) {
		t.Error("过了间隔就该跑")
	}
	// 关掉的不跑
	off := task
	off.Enabled = false
	if Due(off, now, time.Time{}) {
		t.Error("关掉的任务不该跑")
	}
}

func TestDueDailyOnlyOncePerDay(t *testing.T) {
	// 当天 18:00 的任务，现在是 19:00
	now := time.Date(2026, 10, 14, 19, 0, 0, 0, time.Local)
	task := Task{Enabled: true, Kind: KindDaily, Hour: 18, Minute: 0}

	if !Due(task, now, time.Time{}) {
		t.Error("没跑过且已过当天时刻，应该跑")
	}
	// 今天 18:05 跑过 → 今天不再跑
	if Due(task, now, time.Date(2026, 10, 14, 18, 5, 0, 0, time.Local)) {
		t.Error("同一天跑过就不该重复跑")
	}
	// 昨天跑过 → 今天该跑
	if !Due(task, now, time.Date(2026, 10, 13, 18, 0, 0, 0, time.Local)) {
		t.Error("昨天跑过，今天到了时刻就该跑")
	}
	// 还没到今天的时刻
	early := time.Date(2026, 10, 14, 17, 0, 0, 0, time.Local)
	if Due(task, early, time.Date(2026, 10, 13, 18, 0, 0, 0, time.Local)) {
		t.Error("当天时刻还没到，不该跑")
	}
}

func TestUnknownKindNeverDue(t *testing.T) {
	now := time.Now()
	if Due(Task{Enabled: true, Kind: "whatever"}, now, time.Time{}) {
		t.Error("不认识的调度方式不该被判为到期")
	}
	if _, err := NextDue(Task{Kind: "whatever"}, now); err == nil {
		t.Error("不认识的调度方式应该报错")
	}
}
