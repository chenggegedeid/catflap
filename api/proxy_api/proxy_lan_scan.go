package proxy_api

import (
	"github.com/chenggegedeid/catflap/core/lanscan"
	"github.com/chenggegedeid/catflap/middleware"
	"github.com/chenggegedeid/catflap/utils/res"

	"github.com/gin-gonic/gin"
)

// ===================== 反代复用局域网扫描 =====================
//
// 和穿透那边是同一套扫描（core/lanscan），只是填的字段不同：
//   - 穿透：扫出来的机器 + 端口 → 建「服务」
//   - 反代：扫出来的机器 + 端口 → 填站点的 backend
//
// 复用而不是各写一套，是因为"这机器开着哪些端口"这件事只有一个答案 ——
// 两套实现迟早会在探测手法上分叉（超时、并发、要不要试 HTTP）。

// ProxyLanSubnetsView 本机接在哪几个局域网上，给扫描前的下拉框用
func (ProxyApi) ProxyLanSubnetsView(c *gin.Context) {
	subnets := lanscan.ListLanSubnets()
	if subnets == nil {
		subnets = []lanscan.LanSubnet{}
	}
	res.OkWithData(subnets, c)
}

// ProxyLanScanRequest 扫哪一段
type ProxyLanScanRequest struct {
	CIDR string `json:"cidr"`
}

// ProxyLanScanView 扫一个网段，返回活着的机器与开放端口
//
// POST 而不是 GET：扫一段 /24 要几秒到几十秒，这个动作有副作用
// （会向局域网里的机器发起连接），不适合被浏览器或代理预取。
func (ProxyApi) ProxyLanScanView(c *gin.Context) {
	cr := middleware.GetBindRequest[ProxyLanScanRequest](c)

	hosts, err := lanscan.ScanLan(c.Request.Context(), cr.CIDR)
	if err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	if hosts == nil {
		hosts = []lanscan.LanHost{}
	}
	res.OkWithData(hosts, c)
}
