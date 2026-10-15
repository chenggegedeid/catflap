package tunnel_api

import (
	"strings"

	"github.com/chenggegedeid/catflap/middleware"
	"github.com/chenggegedeid/catflap/modules/tunnel"
	"github.com/chenggegedeid/catflap/utils/res"

	"github.com/gin-gonic/gin"
)

// TunnelApi Cloudflare Tunnel
type TunnelApi struct{}

// SaveRequest 保存隧道设置
type SaveRequest struct {
	Enabled    bool   `json:"enabled"`
	BinaryPath string `json:"binaryPath"`
	// Token 留空或掩码表示"不改动"：面板回显的是圆点，用户没重新填时
	// 不该把原来的 token 冲掉。要清空得显式传 clearToken。
	Token      string `json:"token"`
	ClearToken bool   `json:"clearToken"`
	ExtraArgs  string `json:"extraArgs"`
}

// TunnelConfigView 读设置 + 运行状态
func (TunnelApi) TunnelConfigView(c *gin.Context) {
	cfg := tunnel.Runtime.Snapshot()
	res.OkWithData(gin.H{
		"enabled":    cfg.Enabled,
		"binaryPath": cfg.BinaryPath,
		// token 绝不回显，只说填没填
		"tokenSet":  strings.TrimSpace(cfg.Token) != "",
		"extraArgs": cfg.ExtraArgs,
		"state":     tunnel.CurrentState(),
	}, c)
}

// TunnelSaveView 保存并立刻让运行状态跟上。
//
// 保存完就启停：用户点保存就该看到状态变，不用等重启服务。
func (TunnelApi) TunnelSaveView(c *gin.Context) {
	cr := middleware.GetBindRequest[SaveRequest](c)

	err := tunnel.Runtime.Update(func(cfg *tunnel.Config) error {
		cfg.Enabled = cr.Enabled
		cfg.BinaryPath = strings.TrimSpace(cr.BinaryPath)
		cfg.ExtraArgs = strings.TrimSpace(cr.ExtraArgs)
		if cr.ClearToken {
			cfg.Token = ""
			return nil
		}
		if t := strings.TrimSpace(cr.Token); t != "" && !isMasked(t) {
			cfg.Token = t
		}
		return nil
	})
	if err != nil {
		res.FailWithMsg("保存失败："+err.Error(), c)
		return
	}

	tunnel.Sync()
	res.OkWithData(tunnel.CurrentState(), c)
}

// TunnelRestartView 手动重启隧道。
// 改完 CF 那边的配置、或日志里看到"已退出"时用它。
func (TunnelApi) TunnelRestartView(c *gin.Context) {
	tunnel.Stop()
	if err := tunnel.Start(); err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithData(tunnel.CurrentState(), c)
}

// isMasked 面板回显 token 时给的是一串圆点，用户没改就原样提交回来。
// 这种值不能当成真 token 存进去。
func isMasked(s string) bool {
	for _, r := range s {
		if r != '•' && r != '*' {
			return false
		}
	}
	return true
}
