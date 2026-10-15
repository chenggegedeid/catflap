package proxy

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chenggegedeid/catflap/modules/proxy/model"

	"github.com/sirupsen/logrus"
)

// ===================== Cloudflare Tunnel =====================
//
// 为什么要有这个：没有 IPv4 公网（运营商 CGNAT）时，打洞出来的端口是 NAT
// 给的、URL 必须带端口；而 Tunnel 是 cloudflared 主动往外连 Cloudflare，
// 入口在 CF 那边的 443，于是访问地址既没有端口，也不用打洞、不用 DDNS、
// 不用在路由器上放行——这是 CGNAT 环境下最干净的方案。
//
// 这里**不内置 cloudflared**：它是独立程序、体积不小、版本迭代也快，塞进
// 发布包里只会让每次升级都得重新打。做法是托管用户自己装的那一份——
// 填好路径和 token，由 Catflap 拉起、看住、关掉。

const defaultCloudflared = "cloudflared"

// tunnel 隧道进程的运行时状态。整个包里只有一份，用互斥锁护着。
type tunnelRuntime struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	started time.Time
	msg     string
}

var tunnelRt tunnelRuntime

// TunnelState 给面板看的当前状态
func TunnelState() model.TunnelState {
	cfg := currentTunnelConfig()

	tunnelRt.mu.Lock()
	defer tunnelRt.mu.Unlock()

	st := model.TunnelState{
		Running:    tunnelRt.cmd != nil,
		Message:    tunnelRt.msg,
		TokenSet:   strings.TrimSpace(cfg.Token) != "",
		BinaryPath: resolveCloudflared(cfg.BinaryPath),
	}
	if !tunnelRt.started.IsZero() {
		st.StartedAt = tunnelRt.started.Format(time.RFC3339)
	}
	if !cfg.Enabled && st.Message == "" {
		st.Message = "未启用"
	}
	return st
}

func currentTunnelConfig() model.TunnelConfig {
	Runtime.mu.RLock()
	defer Runtime.mu.RUnlock()
	return Runtime.Config.Tunnel
}

// resolveCloudflared 找到要用的 cloudflared。
//
// 填了路径就真的去 PATH 里解析一次（LookPath 会顺带检查它存在且可执行），
// 解析不出来就返回空——这样"填了但其实不存在"能当成"没找到"报出来，而不是
// 等到 exec 启动才炸出一个看不懂的路径错误。没填就按默认名字在 PATH 里找。
func resolveCloudflared(configured string) string {
	if p := strings.TrimSpace(configured); p != "" {
		if abs, err := exec.LookPath(p); err == nil {
			return abs
		}
		return ""
	}
	if p, err := exec.LookPath(defaultCloudflared); err == nil {
		return p
	}
	return ""
}

// StartTunnel 拉起 cloudflared。已经跑着就什么都不做。
func StartTunnel() error {
	cfg := currentTunnelConfig()
	if !cfg.Enabled {
		return errTunnelDisabled
	}

	// 先查 token：它是用户必须自己准备的东西，缺了大概率是还没建隧道；
	// 而二进制没装常常只是路径没配对。先说最可能的那个原因。
	if strings.TrimSpace(cfg.Token) == "" {
		return errNoTunnelToken
	}

	bin := resolveCloudflared(cfg.BinaryPath)
	if bin == "" {
		return errNoCloudflared
	}

	tunnelRt.mu.Lock()
	defer tunnelRt.mu.Unlock()
	if tunnelRt.cmd != nil {
		return nil // 已经在跑
	}

	args := []string{"tunnel", "--no-autoupdate", "run", "--token", strings.TrimSpace(cfg.Token)}
	if extra := strings.Fields(cfg.ExtraArgs); len(extra) > 0 {
		args = append(args, extra...)
	}

	cmd := exec.Command(bin, args...)
	// 只收 stdout/stderr 到日志，不给子进程终端；token 走参数传递，
	// 打日志时整条命令行要避开——下面记的是二进制名而不是完整命令。
	cmd.Stdout = tunnelLogWriter{}
	cmd.Stderr = tunnelLogWriter{}

	if err := cmd.Start(); err != nil {
		tunnelRt.msg = "启动失败: " + err.Error()
		logrus.Errorf("[tunnel] 启动 %s 失败: %v", filepath.Base(bin), err)
		return err
	}

	tunnelRt.cmd = cmd
	tunnelRt.started = time.Now()
	tunnelRt.msg = "隧道已连接"
	logrus.Infof("[tunnel] 已启动 %s", filepath.Base(bin))

	// 子进程退出后清掉记录，让下一次能重新拉起；异常退出要记下来，否则界面上
	// 看着"已连接"其实早断了。
	go func(c *exec.Cmd) {
		err := c.Wait()
		tunnelRt.mu.Lock()
		if tunnelRt.cmd == c {
			tunnelRt.cmd = nil
			if err != nil {
				tunnelRt.msg = "已退出: " + err.Error()
				logrus.Warnf("[tunnel] 进程退出: %v", err)
			} else {
				tunnelRt.msg = "已停止"
			}
		}
		tunnelRt.mu.Unlock()
	}(cmd)

	return nil
}

// StopTunnel 停掉 cloudflared
func StopTunnel() {
	tunnelRt.mu.Lock()
	cmd := tunnelRt.cmd
	tunnelRt.cmd = nil
	tunnelRt.mu.Unlock()

	if cmd == nil || cmd.Process == nil {
		return
	}
	// 先温和地要它自己退（cloudflared 收到 SIGTERM 会优雅断开）
	_ = cmd.Process.Signal(os.Interrupt)
	done := make(chan struct{})
	go func() {
		_, _ = cmd.Process.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
	}
	logrus.Info("[tunnel] 已停止")
}

// SyncTunnel 让运行状态跟上配置：开了就确保跑着，关了就确保停着。
// 配置保存后和程序启动时各调一次。
func SyncTunnel() {
	cfg := currentTunnelConfig()
	if cfg.Enabled {
		if err := StartTunnel(); err != nil && err != errTunnelDisabled {
			logrus.Warnf("[tunnel] 未能启动: %v", err)
		}
		return
	}
	StopTunnel()
}

// tunnelLogWriter 把 cloudflared 的输出接成日志行。
//
// 逐行写而不是攒着：隧道出问题（比如 token 失效）时，用户要看的是"现在
// 报什么"，攒到缓冲里就慢半拍了。
type tunnelLogWriter struct{}

func (tunnelLogWriter) Write(p []byte) (int, error) {
	line := strings.TrimSpace(string(p))
	if line == "" {
		return len(p), nil
	}
	// cloudflared 的输出里可能带 token 之外的敏感信息，但 token 已经在
	// 命令行里传了、输出里一般不含；这里整行原样记，方便排错
	logrus.Infof("[tunnel] %s", line)
	return len(p), nil
}
