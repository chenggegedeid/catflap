package stun_api

import (
	"encoding/json"
	"github.com/chenggegedeid/catflap/modules/stun"

	"github.com/gin-gonic/gin"
)

// GetStunStatusView 返回所有服务的当前运行时快照
//
// Scheduler 可能是 nil：stun 组件还没初始化完（或配置读失败直接返回了），
// 而 HTTP 已经在监听。以前这个接口直接 panic（每次重启都能在日志里看到
// 一条「panic recovered: nil pointer dereference」），前端拿到 500，
// 穿透页在重启后的头几秒是空的。组件没起来就回空列表，别崩。
func (StunApi) GetStunStatusView(c *gin.Context) {
	if stun.Runtime.Scheduler == nil {
		c.JSON(200, []stun.StateEvent{})
		return
	}
	c.JSON(200, stun.Runtime.Scheduler.Snapshot())
}

// StunStatusEventsView SSE 端点：订阅 Scheduler，实时推送 StateEvent
func (StunApi) StunStatusEventsView(c *gin.Context) {
	if stun.Runtime.Scheduler == nil {
		// 组件还没就绪：正常返回，前端的 SSE 会自己重连。
		// 不要在这里 panic —— 见 GetStunStatusView 的说明。
		return
	}
	ch, unsubscribe := stun.Runtime.Scheduler.Subscribe()
	defer unsubscribe()

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no") // 禁止 nginx 缓冲

	// 连接后立即推全量快照，让客户端有初始状态
	for _, event := range stun.Runtime.Scheduler.Snapshot() {
		writeSSEEvent(c, event)
	}
	c.Writer.Flush()

	clientGone := c.Request.Context().Done()
	for {
		select {
		case <-clientGone:
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			writeSSEEvent(c, event)
			c.Writer.Flush()
		}
	}
}

func writeSSEEvent(c *gin.Context, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	c.Writer.WriteString("data: ")
	c.Writer.Write(data)
	c.Writer.WriteString("\n\n")
}
