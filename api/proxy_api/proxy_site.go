package proxy_api

import (
	"strings"
	"time"

	"github.com/chenggegedeid/catflap/middleware"
	"github.com/chenggegedeid/catflap/modules/proxy"
	proxymodel "github.com/chenggegedeid/catflap/modules/proxy/model"
	"github.com/chenggegedeid/catflap/utils/res"

	"github.com/gin-gonic/gin"
)

// SiteSaveRequest 新增 / 修改一个站点，对应 nginx 的一个 server 块。ID 为 0 表示新增。
type SiteSaveRequest struct {
	ID uint `json:"id"`
	// Hosts 域名，如 fn.example.com；支持 *.example.com 通配，可以填多个
	Hosts []string `json:"hosts"`

	PathPrefix  string `json:"pathPrefix"`  // 留空表示整站
	StripPrefix bool   `json:"stripPrefix"` // 转发前剥掉前缀，后端需自己支持 base path

	Backend      string `json:"backend"`      // 192.168.1.20:8080
	BackendHTTPS bool   `json:"backendHttps"` // 后端本身就说 TLS（自签也算）

	// 对外这一侧：不填端口就挂在默认入口上，填了就单独开一个监听
	ListenPort int  `json:"listenPort"` // 0 = 用默认入口
	HTTPS      bool `json:"https"`      // 这个站点对外走不走 HTTPS
	CertID     uint `json:"certId"`     // 0 = 按 SNI 自动匹配

	Enabled     bool   `json:"enabled"`
	Description string `json:"description"`

	// Access IP 门禁：谁能访问这个站点。留空模式 = 谁都能访问。
	Access proxymodel.AccessControl `json:"access"`
}

func (r SiteSaveRequest) toModel() proxymodel.Site {
	return proxymodel.Site{
		ID:           r.ID,
		Hosts:        r.Hosts,
		PathPrefix:   r.PathPrefix,
		StripPrefix:  r.StripPrefix,
		Backend:      r.Backend,
		BackendHTTPS: r.BackendHTTPS,
		ListenPort:   r.ListenPort,
		HTTPS:        r.HTTPS,
		CertID:       r.CertID,
		Enabled:      r.Enabled,
		Description:  r.Description,
		Access:       r.Access,
	}
}

type SiteDeleteRequest struct {
	ID uint `json:"id"`
}

// ProxySiteAddView 新增一个站点
func (ProxyApi) ProxySiteAddView(c *gin.Context) {
	cr := middleware.GetBindRequest[SiteSaveRequest](c)
	cr.ID = 0 // 新增接口不认前端带过来的 ID，免得写成一次静默的覆盖

	saved, err := proxy.SaveSite(cr.toModel())
	if err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithData(saved, c)
}

// ProxySiteUpdateView 修改一个站点
func (ProxyApi) ProxySiteUpdateView(c *gin.Context) {
	cr := middleware.GetBindRequest[SiteSaveRequest](c)
	if cr.ID == 0 {
		res.FailWithMsg("缺少站点 ID", c)
		return
	}

	saved, err := proxy.SaveSite(cr.toModel())
	if err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithData(saved, c)
}

// ProxySiteDeleteView 删除一个站点
func (ProxyApi) ProxySiteDeleteView(c *gin.Context) {
	cr := middleware.GetBindRequest[SiteDeleteRequest](c)
	if err := proxy.DeleteSite(cr.ID); err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithMsg("删除成功", c)
}

// ProxySiteTestView 立即拨一次后端，回报连通性和探到的协议。
//
// 不要求先保存：用户填到一半就能点，省得为了试一下先存一条坏配置。
func (ProxyApi) ProxySiteTestView(c *gin.Context) {
	cr := middleware.GetBindRequest[SiteSaveRequest](c)
	if cr.Backend == "" {
		res.FailWithMsg("请先填写后端地址", c)
		return
	}
	res.OkWithData(proxy.ProbeBackend(cr.toModel()), c)
}

// ===================== 列表上的快捷操作 =====================

// SiteFlagRequest 改站点上一个布尔标记
type SiteFlagRequest struct {
	ID    uint   `json:"id"`
	Value bool   `json:"value"`
	Flag  string `json:"flag"` // "enabled" / "showOnHome"
}

// ProxySiteFlagView 改站点的一个开关（启用 / 显示在主页）。
//
// 为什么单开一个接口而不是走「编辑保存」：列表上一键启停不该为了勾一个框
// 把整个表单打开一遍。
//
// 实现上**复用 SaveSite**（它会统一处理落盘与重开监听），只改一个字段 ——
// 不自己另写一遍配置读写，避免和那边走不同的路径。
func (ProxyApi) ProxySiteFlagView(c *gin.Context) {
	cr := middleware.GetBindRequest[SiteFlagRequest](c)
	if cr.ID == 0 {
		res.FailWithMsg("id 不能为空", c)
		return
	}

	flag := strings.ToLower(strings.TrimSpace(cr.Flag))

	var site *proxymodel.Site
	for i := range proxy.Runtime.Config.Sites {
		if proxy.Runtime.Config.Sites[i].ID == cr.ID {
			site = &proxy.Runtime.Config.Sites[i]
			break
		}
	}
	if site == nil {
		res.FailWithMsg("站点不存在", c)
		return
	}

	switch flag {
	case "enabled":
		site.Enabled = cr.Value
	case "showonhome":
		site.ShowOnHome = cr.Value
	default:
		res.FailWithMsg("不认识的开关", c)
		return
	}
	site.UpdatedAt = time.Now()

	saved, err := proxy.SaveSite(*site)
	if err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithData(saved, c)
}

// ===================== 端口转发 =====================

type ForwardSaveRequest struct {
	ID          uint   `json:"id"` // 0 = 新增
	ListenPort  int    `json:"listenPort"`
	Target      string `json:"target"`
	Protocol    string `json:"protocol"`
	Enabled     bool   `json:"enabled"`
	Description string `json:"description"`
}

type ForwardDeleteRequest struct {
	ID uint `json:"id"`
}

// ProxyForwardSaveView 新增或修改一条端口转发
func (ProxyApi) ProxyForwardSaveView(c *gin.Context) {
	cr := middleware.GetBindRequest[ForwardSaveRequest](c)
	saved, err := proxy.SaveForward(proxymodel.Forward{
		ID:          cr.ID,
		ListenPort:  cr.ListenPort,
		Target:      cr.Target,
		Protocol:    cr.Protocol,
		Enabled:     cr.Enabled,
		Description: cr.Description,
	})
	if err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithData(saved, c)
}

// ProxyForwardDeleteView 删一条端口转发
func (ProxyApi) ProxyForwardDeleteView(c *gin.Context) {
	cr := middleware.GetBindRequest[ForwardDeleteRequest](c)
	if err := proxy.DeleteForward(cr.ID); err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithMsg("已删除", c)
}
