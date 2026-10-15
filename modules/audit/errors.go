package audit

import "errors"

var (
	errEmptyIP     = errors.New("IP 不能为空")
	errBadIP       = errors.New("只支持单个 IP 或 CIDR 网段，如 203.0.113.7 或 203.0.113.0/24")
	errWhitelisted = errors.New("这个地址在白名单里，不会被封禁（要封请先从白名单移除）")
)
