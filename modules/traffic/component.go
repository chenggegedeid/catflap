package traffic

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chenggegedeid/catflap/core"
	"github.com/chenggegedeid/catflap/core/component"

	"github.com/sirupsen/logrus"
)

// Name 组件名
const Name = "traffic"

// enabled 采集开关。用 atomic 是因为它在每个请求的热路径上被读。
var enabled atomic.Bool

// Enabled 采集是否开着
func Enabled() bool { return enabled.Load() }

// SetEnabled 开关采集
func SetEnabled(on bool) { enabled.Store(on) }

// ===================== 站点 ID 的传递 =====================
//
// 流量要按「服务」分组，但 withAccessLog 那层还不认识站点 —— 站点是
// serve() 里 Match 出来的。用 context 把它带上来，比给每层都加参数干净。
//
// 用 context 也顺手让它天然跟着请求生命周期走，不会串。

type siteIDKey struct{}

// WithSiteID 把站点 ID 塞进请求的 context（反代匹配到站点后调）
func WithSiteID(r *http.Request, id uint) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), siteIDKey{}, id))
}

// SiteIDFromRequest 取站点 ID，没有就是 0（未匹配到站点的请求）
func SiteIDFromRequest(r *http.Request) uint {
	if v, ok := r.Context().Value(siteIDKey{}).(uint); ok {
		return v
	}
	return 0
}

// ===================== 生命周期 =====================

var (
	stopOnce sync.Once
	stopCh   chan struct{}
)

// Start 起定时器：每分钟落盘一次分钟记录与累计值
func Start() error {
	if err := Init(); err != nil {
		return err
	}

	// 开一条清理 + 落盘的定时线。用 time.Ticker 而不是 cron 那套：
	// 这里只需要"每分钟做一次"，不需要日历语义。
	stopCh = make(chan struct{})
	ticker := time.NewTicker(time.Minute)

	go func() {
		for {
			select {
			case <-ticker.C:
				Flush()
				if err := SaveTotals(); err != nil {
					logrus.Warnf("[traffic] 累计值落盘失败：%v", err)
				}
				cleanOldFiles()
			case <-stopCh:
				ticker.Stop()
				return
			}
		}
	}()

	enabled.Store(true)
	logrus.Info("[traffic] 流量采集已启动")
	return nil
}

// Stop 停止定时器并做最后一次落盘（退出时调，避免丢最后一分钟的数据）
func Stop() {
	stopOnce.Do(func() {
		if stopCh != nil {
			close(stopCh)
		}
		Flush()
		if err := SaveTotals(); err != nil {
			logrus.Warnf("[traffic] 退出时累计值落盘失败：%v", err)
		}
	})
}

type trafficComponent struct {
	component.LocalSwitch
}

func (trafficComponent) Name() string { return Name }

func (trafficComponent) Init(context.Context) error {
	core.OnShutdown(Stop)
	return Start()
}

func init() {
	component.MustRegister(trafficComponent{
		LocalSwitch: component.NewLocalSwitch(Name),
	})
}
