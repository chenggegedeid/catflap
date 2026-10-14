package ddns

import (
	"github.com/chenggegedeid/catflap/modules/ddns/model"
	"sync"
)

type DDNSRuntime struct {
	mu        sync.RWMutex
	Config    model.DDNSConfig
	Scheduler *Scheduler
}

// CountSyncable 数一数当前有多少条**真正会被同步**的解析记录。
//
// 判据是「记录开着，且能找到它所属的服务商」—— 服务商本身没有开关
// （要么配了要么没配），所以只看记录这侧的开关与归属是否配得上。
// 外部（比如计划任务）据此判断「这一轮到底有没有东西可做」，
// 免得对着一个空调度器报成功。
func (r *DDNSRuntime) CountSyncable() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	known := make(map[uint]bool, len(r.Config.Providers))
	for _, p := range r.Config.Providers {
		known[p.ID] = true
	}

	n := 0
	for _, rec := range r.Config.Records {
		if rec.Enabled && known[rec.ProviderID] {
			n++
		}
	}
	return n
}

var Runtime = &DDNSRuntime{}

// RebuildScheduler 在 providers 变更后调用：停掉旧调度器，按当前配置重建
func (r *DDNSRuntime) RebuildScheduler() {
	schedMu.Lock()
	defer schedMu.Unlock()

	if r.Scheduler != nil {
		r.Scheduler.Stop()
	}

	s := NewScheduler()
	r.Scheduler = s
	s.Start()
	s.Trigger()
}

// StartScheduler 首次建好调度器并跑起来。
//
// 和 RebuildScheduler 共用一把锁：STUN 拿到公网 IP 会从另一个 goroutine 调
// TriggerSync，和这里的赋值是并发的。
func (r *DDNSRuntime) StartScheduler() {
	schedMu.Lock()
	defer schedMu.Unlock()

	if r.Scheduler != nil {
		r.Scheduler.Stop()
	}
	s := NewScheduler()
	r.Scheduler = s
	s.Start()
	s.Trigger()
}

// TriggerSync 触发一次强制全量扫描（“立即同步全部”按钮用）
func (r *DDNSRuntime) TriggerSync() {
	schedMu.Lock()
	s := r.Scheduler
	schedMu.Unlock()
	if s != nil {
		s.Trigger()
	}
}

// noteSyncResult 把一次同步的结果告诉调度器，好决定下次重试的快慢。
// 「单条立即同步」走的不是队列，不主动说一声的话退避计数就一直停在那。
func (r *DDNSRuntime) noteSyncResult(id uint, ok bool) {
	schedMu.Lock()
	s := r.Scheduler
	schedMu.Unlock()
	if s != nil {
		s.noteResult(id, ok)
	}
}
