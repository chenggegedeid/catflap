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
// 流量要按「服务」分组，但采集层（withAccessLog）**在 serve 外面** ——
// 它是外层包装，站点是里面 Match 出来的。
//
// 一开始我用 context 传，结果永远拿到 0：内层 `r = r.WithContext(...)`
// 只改了它那个局部变量，外面的 r 根本没变（context 是不可变的）。
// 所以这里改成**两边共享一个容器**：外层创建、内层填、外层读。

// SiteRef 一次请求里，站点 ID 的共享容器。
//
// 不用 atomic：同一个请求只会被一个 goroutine 处理，写入发生在读取之前
// （都在同一个同步调用链里）。多加同步是没必要的开销。
type SiteRef struct {
	ID uint
}

type siteRefKey struct{}

// NewSiteRef 在外层包装里创建容器并挂进 context，返回新的请求。
func NewSiteRef(r *http.Request) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), siteRefKey{}, &SiteRef{}))
}

// SetSiteID 内层匹配到站点后填进去。
func SetSiteID(r *http.Request, id uint) {
	if ref, ok := r.Context().Value(siteRefKey{}).(*SiteRef); ok {
		ref.ID = id
	}
}

// SiteIDFromRequest 外层读取。没挂容器或没匹配到站点就是 0。
func SiteIDFromRequest(r *http.Request) uint {
	if ref, ok := r.Context().Value(siteRefKey{}).(*SiteRef); ok {
		return ref.ID
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

// Title 见 core/component.Describer。
func (trafficComponent) Title() string { return "流量统计" }

// Summary 见 core/component.Describer。
func (trafficComponent) Summary() string { return "按站点累计进出流量与访问次数" }

func (trafficComponent) Init(context.Context) error {
	core.OnShutdown(Stop)
	return Start()
}

func init() {
	component.MustRegister(trafficComponent{
		LocalSwitch: component.NewLocalSwitch(Name),
	})
}
