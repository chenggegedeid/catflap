package proxy

import (
	"net/http"

	"github.com/chenggegedeid/catflap/modules/audit"

	"github.com/sirupsen/logrus"
)

// writeBlocked 被全局封禁时的响应。
//
// 只说"没有访问权限"，不透露封禁原因与时长 —— 那会告诉扫描器
// "你被识别了、多久后可以再来"，等于给它调参的依据。
func writeBlocked(w http.ResponseWriter, r *http.Request, b audit.BlockEntry) {
	logrus.Infof("[proxy] %s 被全局封禁拒绝（%s）", clientIP(r), b.Reason)
	markDenied(w)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte("403 没有访问权限"))
}
