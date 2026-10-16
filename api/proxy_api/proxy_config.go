package proxy_api

import (
	"github.com/chenggegedeid/catflap/modules/proxy"
	proxymodel "github.com/chenggegedeid/catflap/modules/proxy/model"
	"github.com/chenggegedeid/catflap/utils/res"

	"github.com/gin-gonic/gin"
)

// ProxyConfigView 反向代理页要的全部数据。
//
// 三块：默认入口的设置、此刻实际开着的监听、站点表。
// 对照 nginx 就是「默认 listen 那几行」「netstat 看到的端口」「全部 server 块」。
type ProxyConfigView struct {
	Entry     EntryView           `json:"entry"`
	Listeners []proxy.ListenState `json:"listeners"`
	Sites     []proxymodel.Site   `json:"sites"`
	// Forwards 端口转发。**必须有**：「端口转发」页读的就是这个接口，
	// 少了这个字段，添加能成功但列表永远是空的（用户实测踩到）。
	Forwards []proxymodel.Forward `json:"forwards"`
}

// EntryView 两个默认入口的设置。
//
// 这里是用户的意愿；实际跑没跑起来看 Listeners——
// 端口被占用时就是「开着但没跑起来」，原因在那条 listener 的 lastError 上。
type EntryView struct {
	Enabled bool `json:"enabled"`
	// HTTPPorts / HTTPSPorts 入口端口，**可以多个**。新前端用这两个。
	HTTPPorts  []int `json:"httpPorts"`
	HTTPSPorts []int `json:"httpsPorts"`
	// HTTPPort / HTTPSPort 第一个端口，给老前端兼容用（见 buildEntryView）
	HTTPPort   int    `json:"httpPort"`
	HTTPSPort  int    `json:"httpsPort"`
	CertID     uint   `json:"certId"`
	AccessLog  bool   `json:"accessLog"`
	BaseDomain string `json:"baseDomain"`
}

// ProxyConfigView 取反向代理的配置与状态
func (ProxyApi) ProxyConfigView(c *gin.Context) {
	cfg := proxy.Runtime.Snapshot()
	res.OkWithData(ProxyConfigView{
		Entry:     buildEntryView(cfg),
		Listeners: proxy.ListenStates(),
		// 按界面顺序排好再给前端：拖动排序存在站点的 sortOrder 上，
		// 不排的话拖完一刷新就回到原顺序 —— 表现为「拖动没反应」。
		Sites: proxy.SortSites(cfg.Sites),
		// 端口转发同样要带上：这个视图就是「端口转发」页的数据源。
		Forwards: proxy.SortForwards(cfg.Forwards),
	}, c)
}

func buildEntryView(cfg proxymodel.ProxyConfig) EntryView {
	hp := cfg.HTTPPortList()
	sp := cfg.HTTPSPortList()
	// 顺带回一个单端口值，老前端（只认 httpPort/httpsPort）不会因此崩掉 ——
	// 它读第一个端口，行为和多端口时一致
	var h, hs int
	if len(hp) > 0 {
		h = hp[0]
	}
	if len(sp) > 0 {
		hs = sp[0]
	}
	return EntryView{
		Enabled:    cfg.Enabled,
		HTTPPorts:  hp,
		HTTPSPorts: sp,
		HTTPPort:   h,
		HTTPSPort:  hs,
		CertID:     cfg.CertID,
		AccessLog:  cfg.AccessLog,
		BaseDomain: cfg.BaseDomain,
	}
}
