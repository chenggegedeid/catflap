package task

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/chenggegedeid/catflap/core"
	"github.com/chenggegedeid/catflap/modules/ddns"
	"github.com/chenggegedeid/catflap/modules/wol"

	"github.com/sirupsen/logrus"
)

// 执行动作时可能遇到的几种问题。都做成显式错误而不是就地 log，
// 好让「上次执行结果」能原样显示给用户看。
var (
	// errNoDDNSScheduler DDNS 调度器还没建起来（通常是还没有配置服务商）
	errNoDDNSScheduler = errors.New("DDNS 调度器还没起来：先去 DDNS 页面配一个服务商")
	// errNoDDNSTarget 没有任何「服务商 + 解析记录」的组合可同步
	errNoDDNSTarget = errors.New("还没有配置任何 DDNS 解析记录，本次没有可同步的东西")
	// errWOLTargetGone 关联的那台机器被删掉了。
	//
	// 这种情况不自动删任务：用户可能只是误删，留着让他自己决定。
	// 任务会在每次执行时报错，界面上看得见。
	errWOLTargetGone = errors.New("关联的机器已经不存在了，请重新选择")
	errUnknownAction = errors.New("不认识的动作类型")
)

// TaskRuntime 运行期状态
type TaskRuntime struct {
	mu     sync.RWMutex
	Config Config
	// lastRun 记录每条任务上次运行的时刻，只活在内存里。
	//
	// 不写进配置文件：进程重启后「上次运行时刻」本来就该重新算 ——
	// 半夜关过机的话，写盘的值会让早上开机时立刻补跑一串。
	lastRun map[string]time.Time
	// running 防止同一个任务被并发跑两遍（比如手动触发撞上定时触发）
	running map[string]bool
}

// Runtime 全局运行时
var Runtime = &TaskRuntime{
	lastRun: map[string]time.Time{},
	running: map[string]bool{},
}

// tickInterval 调度器多久检查一次到期
const tickInterval = 30 * time.Second

// InitTask 读取配置、启动调度循环，并注册退出时持久化
func InitTask() error {
	cfg, err := ReadConfig()
	if err != nil {
		return err
	}
	Runtime.mu.Lock()
	Runtime.Config = cfg
	Runtime.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	go loop(ctx)

	core.OnShutdown(func() {
		cancel()
		Runtime.mu.RLock()
		snapshot := Runtime.Config
		Runtime.mu.RUnlock()
		if err := SaveConfig(snapshot); err != nil {
			logrus.Error("保存 task 配置失败：", err)
		}
	})
	return nil
}

func loop(ctx context.Context) {
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			RunDue(time.Now())
		}
	}
}

// RunDue 检查所有任务，跑到期的那些。
//
// 一次只处理一个任务、串行执行：计划任务的量级很小（个位数），
// 并发只会带来「两个动作互相踩」的麻烦（比如同时唤醒和同时同步 DDNS）。
func RunDue(now time.Time) {
	Runtime.mu.RLock()
	tasks := make([]Task, len(Runtime.Config.Tasks))
	copy(tasks, Runtime.Config.Tasks)
	Runtime.mu.RUnlock()

	for _, t := range tasks {
		Runtime.mu.RLock()
		last := Runtime.lastRun[t.ID]
		busy := Runtime.running[t.ID]
		Runtime.mu.RUnlock()

		if busy {
			continue
		}
		if !Due(t, now, last) {
			continue
		}
		RunNow(t.ID)
	}
}

// RunNow 立刻执行一条任务（手动触发也走这里）。
func RunNow(id string) {
	Runtime.mu.RLock()
	var t *Task
	for i := range Runtime.Config.Tasks {
		if Runtime.Config.Tasks[i].ID == id {
			t = &Runtime.Config.Tasks[i]
			break
		}
	}
	Runtime.mu.RUnlock()
	if t == nil {
		return
	}

	Runtime.mu.Lock()
	if Runtime.running[id] {
		Runtime.mu.Unlock()
		return
	}
	Runtime.running[id] = true
	Runtime.mu.Unlock()

	defer func() {
		Runtime.mu.Lock()
		delete(Runtime.running, id)
		Runtime.mu.Unlock()
	}()

	err := execute(*t)
	now := time.Now()

	Runtime.mu.Lock()
	Runtime.lastRun[id] = now
	for i := range Runtime.Config.Tasks {
		if Runtime.Config.Tasks[i].ID == id {
			Runtime.Config.Tasks[i].LastRun = now.Format("2006-01-02 15:04:05")
			if err != nil {
				Runtime.Config.Tasks[i].LastResult = "失败"
				Runtime.Config.Tasks[i].LastError = err.Error()
			} else {
				Runtime.Config.Tasks[i].LastResult = "成功"
				Runtime.Config.Tasks[i].LastError = ""
			}
			break
		}
	}
	snapshot := Runtime.Config
	Runtime.mu.Unlock()

	// 执行结果要落盘，否则重启后界面上「上次结果」是空的
	if err := SaveConfig(snapshot); err != nil {
		logrus.Error("保存 task 执行结果失败：", err)
	}

	if err != nil {
		logrus.Warnf("计划任务 %s 执行失败：%v", t.Name, err)
	} else {
		logrus.Infof("计划任务 %s 执行完成", t.Name)
	}
}

// ListTasks 返回当前配置里的一（份副本）
func ListTasks() []Task {
	Runtime.mu.RLock()
	defer Runtime.mu.RUnlock()
	out := make([]Task, len(Runtime.Config.Tasks))
	copy(out, Runtime.Config.Tasks)
	return out
}

// Snapshot 给 api 层用的一份配置副本
func Snapshot() Config {
	Runtime.mu.RLock()
	defer Runtime.mu.RUnlock()
	cfg := Runtime.Config
	cfg.Tasks = make([]Task, len(Runtime.Config.Tasks))
	copy(cfg.Tasks, Runtime.Config.Tasks)
	return cfg
}

// Persist 写回磁盘并更新运行期
func Persist(cfg Config) error {
	if err := SaveConfig(cfg); err != nil {
		return err
	}
	Runtime.mu.Lock()
	Runtime.Config = cfg
	Runtime.mu.Unlock()
	return nil
}

// execute 真正干活。加动作就是加一个 case。
func execute(t Task) error {
	switch t.Action {
	case ActionDDNS:
		// 复用 ddns 自己的调度器：它能正确处理「provider 变了要重建」
		// 这类状态，自己另起一套同步逻辑只会埋下两套不一致。
		if ddns.Runtime.Scheduler == nil {
			return errNoDDNSScheduler
		}
		// 空调度器的 Trigger 是空操作，会「成功」地什么都不做。
		// 用户配了任务却没配服务商时，界面上看到一片绿，反而以为在正常工作。
		// 这里先数一下有没有真正可同步的对象，没有就明说。
		if n := ddns.Runtime.CountSyncable(); n == 0 {
			return errNoDDNSTarget
		}
		ddns.Runtime.Scheduler.Trigger()
		return nil

	case ActionWOL:
		target, ok := wol.FindTarget(t.Target)
		if !ok {
			return errWOLTargetGone
		}
		return wol.Wake(target.MAC, target.Broadcast, target.Port)

	default:
		return errUnknownAction
	}
}
