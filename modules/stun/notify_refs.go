package stun

import (
	"time"

	"github.com/sirupsen/logrus"
)

// CleanupDeletedChannel 通知中心删了某条渠道时调。
//
// 服务只是引用了渠道 ID，渠道没了这个 ID 留着就是悬空 —
// 穿透成功发通知时 GetChannel(id) 找不到，会跳过（hasNotifyCenter 那条路径
// 走不到就会变静默失败，连日志都没有）。所以**渠道删除必须联动清服务**，
// 否则用户只看到"渠道列表少了一条"，但服务还在引用它，会很奇怪。
func CleanupDeletedChannel(id string) error {
	if id == "" {
		return nil
	}
	cleaned := 0
	for i := range Runtime.Config.Devices {
		for j := range Runtime.Config.Devices[i].Services {
			svc := &Runtime.Config.Devices[i].Services[j]
			kept := make([]string, 0, len(svc.ChannelIDs))
			changed := false
			for _, cid := range svc.ChannelIDs {
				if cid == id {
					changed = true
					continue
				}
				kept = append(kept, cid)
			}
			if changed {
				svc.ChannelIDs = kept
				svc.UpdatedAt = now()
				cleaned++
			}
		}
	}
	if cleaned == 0 {
		return nil
	}
	// 落盘失败要往上抛：调用方（删渠道的接口）如果拿到"成功"，
	// 界面上渠道已经消失了，服务里却还留着悬空 ID ——
	// 用户看到的是"删干净了"，实际是两边不一致，而且没有任何报错。
	if err := UpdateConfig(Runtime.Config); err != nil {
		logrus.Warnf("[stun] 清理已删渠道 %s 的引用失败：%v", id, err)
		return err
	}
	logrus.Infof("[stun] 已从 %d 个服务的通知引用中清理掉已删渠道 %s", cleaned, id)
	return nil
}

// now 抽出来留个钩子方便后续做依赖注入。
var now = func() time.Time { return time.Now() }

// ServiceChannelRef 一个服务对通知渠道的引用（汇总给通知中心页）
type ServiceChannelRef struct {
	DeviceID    uint     `json:"deviceId"`
	ServiceID   uint     `json:"serviceId"`
	ServiceName string   `json:"serviceName"`
	ChannelIDs  []string `json:"channelIds"`
}

// ServiceChannelRefs 列出所有"启用了通知"的服务 —— 给通知中心页聚合展示用。
//
// 通知中心是全局的，不在穿透服务的页里能看出"我配了哪些服务"；
// 这里给它一个列表，通知页能直接看到"这些服务在用你配的渠道"。
// 不带服务的 channelIds（空切片）也返回 —— 让用户看到"这个服务**没**开通知"也是有用的。
func ServiceChannelRefs() []ServiceChannelRef {
	var out []ServiceChannelRef
	for i := range Runtime.Config.Devices {
		for j := range Runtime.Config.Devices[i].Services {
			svc := &Runtime.Config.Devices[i].Services[j]
			ids := svc.ChannelIDs
			if ids == nil {
				ids = []string{}
			}
			out = append(out, ServiceChannelRef{
				DeviceID:    Runtime.Config.Devices[i].DeviceID,
				ServiceID:   svc.ID,
				ServiceName: svc.Name,
				ChannelIDs:  ids,
			})
		}
	}
	return out
}
