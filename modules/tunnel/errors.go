package tunnel

import "errors"

var (
	// errTunnelDisabled 没启用隧道。SyncTunnel 里不算错误，静默跳过即可。
	errTunnelDisabled = errors.New("隧道未启用")
	errNoCloudflared  = errors.New("没找到 cloudflared：请先安装它，或在设置里填上它的完整路径")
	errNoTunnelToken  = errors.New("还没填隧道 token：在 Cloudflare Zero Trust 后台创建隧道时会给一长串，粘进来即可")
)
