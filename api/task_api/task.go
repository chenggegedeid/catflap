package task_api

import (
	"fmt"
	"time"

	"github.com/chenggegedeid/catflap/middleware"
	"github.com/chenggegedeid/catflap/modules/task"
	"github.com/chenggegedeid/catflap/utils/res"

	"github.com/gin-gonic/gin"
)

// TaskApi 计划任务接口
type TaskApi struct{}

// TaskResponse 返回给前端的一条任务
type TaskResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Enabled     bool   `json:"enabled"`
	Kind        string `json:"kind"`
	IntervalMin int    `json:"intervalMin"`
	Hour        int    `json:"hour"`
	Minute      int    `json:"minute"`
	Action      string `json:"action"`
	Target      string `json:"target"`
	LastRun     string `json:"lastRun,omitempty"`
	LastResult  string `json:"lastResult,omitempty"`
	LastError   string `json:"lastError,omitempty"`
	// NextRun 下一次预计执行时刻，由后端算好给前端显示
	NextRun string `json:"nextRun,omitempty"`
}

// TaskListResponse 列表
type TaskListResponse struct {
	Tasks []TaskResponse `json:"tasks"`
}

func toResponse(t task.Task) TaskResponse {
	out := TaskResponse{
		ID:          t.ID,
		Name:        t.Name,
		Enabled:     t.Enabled,
		Kind:        string(t.Kind),
		IntervalMin: t.IntervalMin,
		Hour:        t.Hour,
		Minute:      t.Minute,
		Action:      string(t.Action),
		Target:      t.Target,
		LastRun:     t.LastRun,
		LastResult:  t.LastResult,
		LastError:   t.LastError,
	}
	if !t.Enabled {
		return out
	}
	// 下一次时刻：interval 用「上次跑 + 间隔」，没跑过就算「现在 + 间隔」
	now := time.Now()
	if next, err := task.NextDue(t, now); err == nil {
		out.NextRun = next.Format("2006-01-02 15:04:05")
	}
	return out
}

// TaskListView 列出所有任务
func (TaskApi) TaskListView(c *gin.Context) {
	list := task.ListTasks()
	out := make([]TaskResponse, 0, len(list))
	for _, t := range list {
		out = append(out, toResponse(t))
	}
	res.OkWithData(TaskListResponse{Tasks: out}, c)
}

// TaskSaveRequest 新增/修改。ID 为空表示新增。
type TaskSaveRequest struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Enabled     bool   `json:"enabled"`
	Kind        string `json:"kind"`
	IntervalMin int    `json:"intervalMin"`
	Hour        int    `json:"hour"`
	Minute      int    `json:"minute"`
	Action      string `json:"action"`
	Target      string `json:"target"`
}

func (r TaskSaveRequest) toTask() (task.Task, error) {
	t := task.Task{
		ID:          r.ID,
		Name:        r.Name,
		Enabled:     r.Enabled,
		Kind:        task.ScheduleKind(r.Kind),
		IntervalMin: r.IntervalMin,
		Hour:        r.Hour,
		Minute:      r.Minute,
		Action:      task.ActionKind(r.Action),
		Target:      r.Target,
		CreatedAt:   time.Now(),
	}
	if t.Name == "" {
		return t, fmt.Errorf("名称不能为空")
	}
	// 复用 NextDue 做一次校验，调度参数不合法在这里就被挡住，
	// 不用等到跑的时候才发现（那时只会静默跳过，用户一头雾水）
	if _, err := task.NextDue(t, time.Now()); err != nil {
		return t, err
	}
	switch t.Action {
	case task.ActionDDNS, task.ActionWOL:
	default:
		return t, fmt.Errorf("不认识的动作 %q", r.Action)
	}
	if t.Action == task.ActionWOL && t.Target == "" {
		return t, fmt.Errorf("唤醒动作要选一台机器")
	}
	return t, nil
}

// TaskAddView 新增
func (TaskApi) TaskAddView(c *gin.Context) {
	cr := middleware.GetBindRequest[TaskSaveRequest](c)
	t, err := cr.toTask()
	if err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	t.ID = fmt.Sprintf("task-%d", time.Now().UnixNano())

	cfg := task.Snapshot()
	cfg.Tasks = append(cfg.Tasks, t)
	if err := task.Persist(cfg); err != nil {
		res.FailWithMsg("保存失败", c)
		return
	}
	res.OkWithData(toResponse(t), c)
}

// TaskUpdateView 修改
func (TaskApi) TaskUpdateView(c *gin.Context) {
	cr := middleware.GetBindRequest[TaskSaveRequest](c)
	if cr.ID == "" {
		res.FailWithMsg("缺少 id", c)
		return
	}
	next, err := cr.toTask()
	if err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}

	cfg := task.Snapshot()
	idx := -1
	for i := range cfg.Tasks {
		if cfg.Tasks[i].ID == cr.ID {
			idx = i
			break
		}
	}
	if idx == -1 {
		res.FailWithMsg("找不到这条任务", c)
		return
	}
	// 保留创建时间与运行结果，只覆盖用户能改的字段
	next.CreatedAt = cfg.Tasks[idx].CreatedAt
	next.LastRun = cfg.Tasks[idx].LastRun
	next.LastResult = cfg.Tasks[idx].LastResult
	next.LastError = cfg.Tasks[idx].LastError
	cfg.Tasks[idx] = next

	if err := task.Persist(cfg); err != nil {
		res.FailWithMsg("保存失败", c)
		return
	}
	res.OkWithData(toResponse(next), c)
}

// TaskDeleteRequest 删除
type TaskDeleteRequest struct {
	ID string `json:"id"`
}

func (TaskApi) TaskDeleteView(c *gin.Context) {
	cr := middleware.GetBindRequest[TaskDeleteRequest](c)
	cfg := task.Snapshot()
	next := make([]task.Task, 0, len(cfg.Tasks))
	found := false
	for _, t := range cfg.Tasks {
		if t.ID == cr.ID {
			found = true
			continue
		}
		next = append(next, t)
	}
	if !found {
		res.FailWithMsg("找不到这条任务", c)
		return
	}
	cfg.Tasks = next
	if err := task.Persist(cfg); err != nil {
		res.FailWithMsg("保存失败", c)
		return
	}
	res.OkWithMsg("已删除", c)
}

// TaskRunRequest 手动触发
type TaskRunRequest struct {
	ID string `json:"id"`
}

// TaskRunView 立刻跑一次。
//
// 与定时触发走同一条路径（RunNow），所以「手动能跑通、定时没反应」
// 这种情况不会出现 —— 排查时两边行为一致。
func (TaskApi) TaskRunView(c *gin.Context) {
	cr := middleware.GetBindRequest[TaskRunRequest](c)
	var found bool
	for _, t := range task.ListTasks() {
		if t.ID == cr.ID {
			found = true
			break
		}
	}
	if !found {
		res.FailWithMsg("找不到这条任务", c)
		return
	}
	// 同步执行：动作本身是秒级的（触发一次 DDNS / 发一个唤醒包），
	// 让前端直接拿到结果比「提交成功，请稍后刷新」清楚得多
	task.RunNow(cr.ID)

	for _, t := range task.ListTasks() {
		if t.ID == cr.ID {
			if t.LastError != "" {
				res.OkWithData(gin.H{"ok": false, "error": t.LastError}, c)
				return
			}
			res.OkWithData(gin.H{"ok": true, "lastRun": t.LastRun}, c)
			return
		}
	}
	res.OkWithMsg("已执行", c)
}
