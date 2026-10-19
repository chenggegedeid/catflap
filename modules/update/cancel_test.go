package update

import (
	"context"
	"testing"
	"time"
)

// 这几条守的是「更新卡住出不来」的修复。
//
// 用户实际遇到的现象：上传显示"已经在更新了"，刷新页面看不到进度，
// 再点更新还是"已经在更新了" —— 任务卡在 downloading / restarting，
// 既不能取消也不能重来，只能重启整个 Catflap。

// TestCancelJobWhileDownloading 下载中取消：状态立刻回空闲，能重新发起。
func TestCancelJobWhileDownloading(t *testing.T) {
	r := &UpdateRuntime{}
	r.job = Job{State: "downloading", Total: 100}
	r.done.Store(42)
	var cancelled bool
	r.jobCancel = func() { cancelled = true }

	if err := r.CancelJob(); err != nil {
		t.Fatalf("下载中应当可以取消，实际报错：%v", err)
	}
	if !cancelled {
		t.Error("取消时应当调用下载那边的 cancel，否则阻塞的 Read 不会醒")
	}
	if r.job.State != "" {
		t.Errorf("取消后应当回到空闲，实际 %q", r.job.State)
	}
	if r.done.Load() != 0 {
		t.Errorf("取消后进度应当清零，实际 %d", r.done.Load())
	}
	if r.jobCancel != nil {
		t.Error("取消后不该再留着已经作废的 cancel")
	}
}

// TestCancelJobClearsStuckRestarting 卡在"正在重启"也能清掉。
//
// 这是最要命的一种：自动重启没拉起来时状态停在这儿，
// 不取消的话连手动上传都会被"已经在更新了"挡回去。
func TestCancelJobClearsStuckRestarting(t *testing.T) {
	r := &UpdateRuntime{}
	r.job = Job{State: "restarting"}

	if err := r.CancelJob(); err != nil {
		t.Fatalf("卡在重启中时也应当能清掉，实际报错：%v", err)
	}
	if r.job.State != "" {
		t.Errorf("清掉后应当回到空闲，实际 %q", r.job.State)
	}
}

// TestCancelJobWhenIdle 空闲时取消要给明确反馈，而不是静默成功。
func TestCancelJobWhenIdle(t *testing.T) {
	r := &UpdateRuntime{}
	if err := r.CancelJob(); err == nil {
		t.Error("没有正在进行的更新时，取消应当返回错误（界面要提示）")
	}
}

// TestCancelThenInstallAgain 取消之后必须能重新开始 —— 这是用户真正要的。
func TestCancelThenInstallAgain(t *testing.T) {
	r := &UpdateRuntime{}
	r.job = Job{State: "downloading"}
	_ = r.CancelJob()

	// 重新发起的前置检查就是这个条件，取消后它必须为假
	blocked := r.job.State == "downloading" || r.job.State == "restarting"
	if blocked {
		t.Error("取消之后仍然被判为「已经在更新了」，等于没取消掉")
	}
}

// TestDownloadStopsOnCancel ctx 取消后，下载不再继续换下一个源。
//
// 每个源都试一遍的话，取消要等所有源都超时才生效，用户会觉得"点了没反应"。
func TestDownloadStopsOnCancel(t *testing.T) {
	// 用一个已经取消的 ctx：应当立刻返回，而不是去连外部的源
	r := &UpdateRuntime{}
	done := make(chan error, 1)
	go func() {
		done <- r.download(canceledCtx(), releaseAsset{
			Name:        "catflap",
			DownloadURL: "https://127.0.0.1:1/never",
			Size:        1,
		}, []string{"a", "b", "c"})
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Error("ctx 已取消时不该返回成功")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("ctx 已取消，download 却还在跑（没检查取消就换下一个源了）")
	}
}

// canceledCtx 一个已经取消掉的 context，给上面的测试用
func canceledCtx() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}
