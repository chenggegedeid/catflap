//go:build windows

package update

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// replaceExecutable Windows 上正在运行的 exe 不能覆盖、不能删，但能改名：
// 先把自己挪成 .old，再把新文件放到原位，.old 等下次启动时清掉
func replaceExecutable(exe, tmp string) error {
	old := exe + ".old"
	_ = os.Remove(old)
	if err := os.Rename(exe, old); err != nil {
		return fmt.Errorf("挪开旧程序失败，请手动替换：%w", err)
	}
	if err := os.Rename(tmp, exe); err != nil {
		_ = os.Rename(old, exe) // 放回去，至少下次还能启动
		return fmt.Errorf("放置新程序失败，请手动替换：%w", err)
	}
	return nil
}

// restart 托一个隐藏的 PowerShell 等本进程退干净再启动新程序，然后自己退出。
//
// 不能直接先拉起新进程：它一探测 3791 还在就去复用这个马上要退的后端，
// 桌面版还会被 Wails 单实例当成第二个实例直接退掉。
// 也不能让新进程自己等旧的——装上去的可能是不认得这一套的老版本（手动上传回退），
// 所以等待放在外面，换成哪个版本都能起来
func restart() error {
	exe, err := executablePath()
	if err != nil {
		return err
	}
	wd, _ := os.Getwd()
	args := ""
	if len(os.Args) > 1 {
		args = " -ArgumentList " + strings.Join(quoteAll(os.Args[1:]), ",")
	}
	script := fmt.Sprintf(
		"Wait-Process -Id %d -Timeout 60 -ErrorAction SilentlyContinue; Start-Process -FilePath %s -WorkingDirectory %s%s",
		os.Getpid(), psQuote(exe), psQuote(wd), args, // 配置在工作目录下，换了目录就像配置丢了
	)
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("拉起重启脚本失败：%w", err)
	}
	_ = cmd.Process.Release()
	quit()
	return nil
}

func psQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func quoteAll(ss []string) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = psQuote(s)
	}
	return out
}

// removeOldExecutable 清掉上次更新挪开的旧程序。这时旧进程早退了，删得掉
func removeOldExecutable() {
	if exe, err := executablePath(); err == nil {
		_ = os.Remove(exe + ".old")
	}
}
