//go:build !desktop

package main

import (
	"fmt"
	"os"

	"github.com/chenggegedeid/catflap/core"
	"github.com/sirupsen/logrus"
)

func main() {
	// 参数必须在 initRuntime() **之前**处理。
	//
	// 以前这里没有任何参数解析：`catflap --version` 不会打印版本，
	// 而是被当成「不需要参数」直接把后端跑起来（日志里出现
	// "Catflap Run / 后端已在 xxx 运行"）。两个后果：
	//   1. 装完之后没法问机器上装的是哪一版 —— 安装脚本要做「已安装 →
	//      更新还是卸载」的判断，拿不到版本号就只能靠猜；
	//   2. 在已经有实例在跑的机器上敲 --version，会看到一串启动日志然后
	//      程序退出（端口占用），非常像崩了。
	//
	// 所以先认参数，认出来就办完事立刻退出，绝不往下走到启动流程。
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--version", "-v", "version":
			// 只打印版本号（不带 v 前缀由注入方决定，这里原样输出）
			fmt.Println(core.Version)
			return
		case "--help", "-h":
			fmt.Print(`Catflap —— 家宽内网穿透与自托管入口

用法：
  catflap              启动后台服务（默认）
  catflap --version    打印版本号后退出
  catflap --help       显示本帮助

服务由 systemd 托管时一般不用手动启动：
  systemctl start catflap
  journalctl -u catflap -f
`)
			return
		case "--":
			// 显式分隔符，后面的一律不管
		default:
			// 未知参数不猜：直接说清楚，别默默地把它当成"启动"。
			// 比悄悄跑起来好 —— 用户能立刻发现自己敲错了。
			logrus.Warnf("不认识的参数 %q，已忽略并继续启动（--help 看用法）", os.Args[1])
		}
	}

	initRuntime()

	started, err := startBackend(webFS)
	if err != nil {
		logrus.Fatal(err)
	}
	if !started {
		logrus.Infof("Catflap 已在 %s 运行", adminURL)
		return
	}

	postStartup()

	logrus.Infof("管理后台运行在 %s", adminURL)
	logrus.Infof("导航主页运行在 %s", homeURL)
	select {}
}
