//go:build !windows

package update

import (
	"fmt"
	"os"
	"syscall"

	"github.com/chenggegedeid/catflap/core"

	"github.com/sirupsen/logrus"
)

// replaceExecutable 同目录 rename 是原子的，正在跑的进程拿着旧 inode 不受影响
func replaceExecutable(exe, tmp string) error {
	if err := os.Rename(tmp, exe); err != nil {
		return fmt.Errorf("替换程序失败，请手动替换：%w", err)
	}
	return nil
}

// restart 保存配置后 exec 成新程序。PID 不变，systemd / procd 看来进程一直在，
// 前台跑的也还在原来的终端里。监听的端口 Go 都设了 CLOEXEC，exec 时自动关掉
func restart() error {
	exe, err := executablePath()
	if err != nil {
		return err
	}
	core.RunShutdown()
	err = syscall.Exec(exe, os.Args, os.Environ())
	// 走到这里说明 exec 失败了，而配置已经保存、反向代理已经停了，留着也是半死不活：
	// 直接以失败退出，systemd（Restart=on-failure）/ procd 会用新文件拉起来
	logrus.Errorf("重启成新版本失败：%v", err)
	os.Exit(1)
	return nil
}

// removeOldExecutable 非 Windows 是原地 rename，没有旧文件要清
func removeOldExecutable() {}
