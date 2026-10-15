package proxy

import (
	"net"
	"net/http"

	"github.com/chenggegedeid/catflap/modules/proxy/model"

	"github.com/sirupsen/logrus"
)

// accessAllowed 这个来源能不能访问该站点
//
// 来源 IP 用的就是 accesslog.go 里的 clientIP：同样只认 TCP 对端地址，
// 不读 X-Forwarded-For / X-Real-IP —— 那些头是客户端想写什么就写什么的，
// 在门口没有可信反代的情况下拿它做准入判断，等于把门禁钥匙交给来访的人
// （伪造一个 X-Forwarded-For: 8.8.8.8 就进来了）。
//
// 注意：如果站点前面确实挂了可信反代（Cloudflare 之类），这里拿到的是反代的
// IP，白名单得填反代的网段才有效。这种情况建议直接在反代那侧做访问控制。
func accessAllowed(site model.Site, r *http.Request) bool {
	ac := site.Access
	if ac.Mode == model.AccessModeAllowAll || len(ac.Rules) == 0 {
		// 没配就是全放行。空规则配了模式也按放行处理——否则手滑勾了白名单
		// 却没填内容，会把所有人都关在外面，包括自己。
		return true
	}

	ip := net.ParseIP(clientIP(r))
	if ip == nil {
		// 解析不出来就按白名单拒绝、黑名单放行处理：拿不准时宁可挡住，
		// 但黑名单的语义是"只挡列出来的"，不该把看不懂的也挡了。
		return ac.Mode == model.AccessModeDeny
	}

	matched := false
	for _, rule := range ac.Rules {
		rule = trimSpace(rule)
		if rule == "" {
			continue
		}
		if _, cidr, err := net.ParseCIDR(rule); err == nil {
			if cidr.Contains(ip) {
				matched = true
				break
			}
			continue
		}
		if single := net.ParseIP(rule); single != nil && single.Equal(ip) {
			matched = true
			break
		}
	}

	if ac.Mode == model.AccessModeAllow {
		return matched
	}
	return !matched
}

// writeAccessDenied 门禁没过时的响应。
//
// 只说"没有访问权限"，不透露这个站点存在不存在、后面是什么——
// 那正是访问控制要藏的东西。
func writeAccessDenied(w http.ResponseWriter, site model.Site, r *http.Request) {
	logrus.Infof("[proxy] %s 拒绝访问：来源 %s 不在允许范围内", site.Label(), clientIP(r))
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte("403 没有访问权限"))
}

func trimSpace(s string) string {
	// 不想为这一个函数引 strings 包进来污染 import，手写足够：规则是用户填的
	// IP / CIDR，前后偶尔带空格或换行，剥掉即可
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t' || s[0] == '\n' || s[0] == '\r') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t' || s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}
