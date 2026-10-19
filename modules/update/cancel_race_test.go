package update

import (
	"context"
	"errors"
	"testing"
	"time"
)

// 取消与 goroutine 收尾的竞态：CancelJob 清空 job 后，
// 下载 goroutine 结束时只能把它**再清空一次**，绝不能写成 failed/restarting。
func TestCancelNotOverwrittenByGoroutine(t *testing.T) {
	r := &UpdateRuntime{}
	r.job = Job{State: "downloading", Total: 999}
	r.done.Store(10)

	ctx, cancel := context.WithCancel(context.Background())
	r.jobCancel = cancel
	r.jobGen++
	gen := r.jobGen

	// 模拟 goroutine：取消后收到 Canceled
	go func() {
		<-ctx.Done()
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.jobGen == gen {
			r.jobCancel = nil
		}
		err := ctx.Err() // Canceled
		if errors.Is(err, context.Canceled) {
			r.job = Job{}
			r.done.Store(0)
			return
		}
		r.job.State = "failed"
	}()

	if err := r.CancelJob(); err != nil {
		t.Fatalf("取消失败：%v", err)
	}
	// 等 goroutine 收尾
	for i := 0; i < 50; i++ {
		r.mu.RLock()
		s := r.job.State
		r.mu.RUnlock()
		if s == "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.job.State != "" {
		t.Errorf("取消后不该被 goroutine 改成 %q（应回到空闲）", r.job.State)
	}
}
