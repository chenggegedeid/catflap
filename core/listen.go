package core

import (
	"fmt"
	"os"
	"strconv"

	"github.com/sirupsen/logrus"
)

// DefaultPort 面板监听的端口。
//
// 为什么不是 3333：那个端口被太多东西当默认值（各类 dev server、面板、
// 调试工具），撞上了就只能去把占着它的程序挪开 —— 而本程序又是个开机自启的
// 常驻服务，起不来用户往往隔很久才发现。挑一个不常用的四位数，撞的概率低得多。
//
// 为什么还要留环境变量（PortEnv）：换端口只能降低概率，不能消除。真撞上时
// 必须给用户一条不改代码就能走的路，否则「端口被占」就是个死结。
const DefaultPort = 3791

// PortEnv 改监听端口的环境变量名。
const PortEnv = "CATFLAP_PORT"

// ListenPort 当前监听端口：环境变量给了合法值就用它，否则用 DefaultPort。
//
// 环境变量不是合法端口（不是数字、越界）时**记录警告并回落默认值**，而不是
// 启动失败：端口不对的情况下程序还能起来、日志里也写着实际用的端口，用户
// 看一眼就知道；而启动失败连日志都未必看得到，排查成本高得多。
func ListenPort() int {
	raw := os.Getenv(PortEnv)
	if raw == "" {
		return DefaultPort
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > 65535 {
		logrus.Warnf("环境变量 %s=%q 不是合法端口，改用默认端口 %d", PortEnv, raw, DefaultPort)
		return DefaultPort
	}
	return n
}

// ListenAddr 监听地址（绑 0.0.0.0，局域网其它设备也要能访问面板）。
func ListenAddr() string {
	return fmt.Sprintf("0.0.0.0:%d", ListenPort())
}

// LoopbackAddr 本机回环地址，用于「后端起来了没有」的探测。
func LoopbackAddr() string {
	return fmt.Sprintf("127.0.0.1:%d", ListenPort())
}

// LoopbackURL 本机回环的 base URL，后台与导航主页的地址都由它拼出来。
func LoopbackURL() string {
	return "http://" + LoopbackAddr()
}
