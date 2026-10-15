package stun

import (
	"strconv"
	"time"

	"github.com/chenggegedeid/catflap/modules/webhook"

	"github.com/sirupsen/logrus"
)

// ===================== 供通知中心迁移用的只读/回写接口 =====================
//
// 通知中心要把「每个服务内联的 webhook 配置」收成全局渠道，但**不能让
// notify 模块去翻 stun 的内部结构** —— 那就变成业务模块互相依赖了。
//
// 所以由 stun 这边主动提供两个函数：一个读（列出旧配置），一个写（回写渠道引用）。
// 依赖方向是 notify → stun，而 stun 完全不知道 notify 的存在（参数是普通类型）。

// LegacyWebhook 一处旧配置
type LegacyWebhook struct {
	DeviceID    uint
	ServiceID   uint
	ServiceName string
	Config      webhook.WebhookConfig
}

// LegacyWebhooks 列出所有「还配着内联 webhook」的服务。
//
// 只列**真正配过的**（URL 非空）：一个新服务默认就带个空配置，
// 把那也当"配过"会迁移出一堆空渠道。
func LegacyWebhooks() []LegacyWebhook {
	var out []LegacyWebhook

	for _, dev := range Runtime.Config.Devices {
		for _, svc := range dev.Services {
			if svc.WebHookConfig.URL == "" {
				continue
			}
			out = append(out, LegacyWebhook{
				DeviceID:    dev.DeviceID,
				ServiceID:   svc.ID,
				ServiceName: svc.Name,
				Config:      svc.WebHookConfig,
			})
		}
	}
	return out
}

// ApplyChannelRefs 通知中心迁移完之后，把服务的引用写回去。
//
// refs 的键是 "deviceID:serviceID"，值是新渠道 ID。
// 写完之后服务的通知走通知中心，`WebHookConfig` 字段留着不动 ——
// 那是回退用的（打开 notify 的 LegacyWebhookFallback 就回到旧路径）。
func ApplyChannelRefs(refs map[string]string) error {
	if len(refs) == 0 {
		return nil
	}

	for i := range Runtime.Config.Devices {
		dev := &Runtime.Config.Devices[i]
		for j := range dev.Services {
			svc := &dev.Services[j]
			id, ok := refs[keyOf(dev.DeviceID, svc.ID)]
			if !ok {
				continue
			}
			// 去重追加：迁移可能因为重跑而走到这里两次
			found := false
			for _, existing := range svc.ChannelIDs {
				if existing == id {
					found = true
					break
				}
			}
			if !found {
				svc.ChannelIDs = append(svc.ChannelIDs, id)
			}
			svc.UpdatedAt = time.Now()
		}
	}

	if err := UpdateConfig(Runtime.Config); err != nil {
		return err
	}
	logrus.Infof("[stun] 已为 %d 个服务写入通知渠道引用", len(refs))
	return nil
}

// ServiceChannels 这个服务该往哪些渠道发通知。
//
// 优先用新的渠道引用（ChannelIDs）；一条都没有时说明迁移还没跑或用户没配，
// 此时返回空 —— 由通知中心那边决定要不要回退读旧字段。
func ServiceChannels(deviceID, serviceID uint) []string {
	for _, dev := range Runtime.Config.Devices {
		if dev.DeviceID != deviceID {
			continue
		}
		for _, svc := range dev.Services {
			if svc.ID != serviceID {
				continue
			}
			return append([]string{}, svc.ChannelIDs...)
		}
	}
	return nil
}

func keyOf(deviceID, serviceID uint) string {
	return strconv.FormatUint(uint64(deviceID), 10) + ":" + strconv.FormatUint(uint64(serviceID), 10)
}
