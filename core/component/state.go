package component

import (
	"encoding/json"
	"os"
	"sort"
	"sync"

	"github.com/chenggegedeid/catflap/utils/utilsFile"

	"github.com/sirupsen/logrus"
)

// StatePath 组件开关状态的落盘位置。
//
// 是 var 而不是 const：测试要把它指到临时目录，跑测试不该动开发者本机的
// config/。生产代码不要改它。
//
// 与其它配置放一起，一起受 config/ 的 0700/0600 权限保护 —— 这份状态虽然
// 不含凭据，但「关掉了哪个功能」也是部署信息，没必要给同机其它用户看。
var StatePath = "config/components.json"

// stateFile 落盘结构。
//
// 存的是**被关掉的**那一组，而不是「开着的」：新加的组件默认就该是开的。
// 存白名单的话，每次新增组件都要往老用户的文件里补一笔，忘了就是
// 「新版本装上了但这个功能不生效」——这种问题用户基本查不出来。
type stateFile struct {
	Disabled []string `json:"disabled"`
}

type switchState struct {
	mu       sync.RWMutex
	loaded   bool
	disabled map[string]bool
}

var switches = &switchState{}

// ensureLoaded 首次访问时读一次盘。
//
// 用「懒加载 + 双检」而不是在 init() 里读：包级 init 的执行时机取决于
// 谁先 import，而配置目录是否可读、要不要建目录这些事最好在一个确定的位置发生。
// 好处还有一个：单元测试里不需要任何准备就能用（读不到文件就按全开）。
func (s *switchState) ensureLoaded() {
	s.mu.RLock()
	loaded := s.loaded
	s.mu.RUnlock()
	if loaded {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loaded {
		return
	}
	s.loadLocked()
}

// loadLocked 读盘。调用方需持有写锁。
func (s *switchState) loadLocked() {
	s.loaded = true
	s.disabled = map[string]bool{}

	data, err := os.ReadFile(StatePath)
	if err != nil {
		// 首次启动没有这个文件，是正常的
		if !os.IsNotExist(err) {
			logrus.Warnf("读取组件开关状态失败，本次按全部启用处理：%v", err)
		}
		return
	}

	var f stateFile
	if err := json.Unmarshal(data, &f); err != nil {
		// 文件坏了也不该让整个面板起不来：功能全开总比全不可用强
		logrus.Warnf("组件开关状态文件解析失败，本次按全部启用处理：%v", err)
		return
	}
	for _, n := range f.Disabled {
		if n != "" {
			s.disabled[n] = true
		}
	}
}

func (s *switchState) isEnabled(name string) bool {
	s.ensureLoaded()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return !s.disabled[name]
}

func (s *switchState) setEnabled(name string, on bool) error {
	s.ensureLoaded()

	s.mu.Lock()
	defer s.mu.Unlock()

	if on {
		delete(s.disabled, name)
	} else {
		s.disabled[name] = true
	}
	return s.persistLocked()
}

func (s *switchState) disabledNames() []string {
	s.ensureLoaded()
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.disabled))
	for n := range s.disabled {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// persistLocked 把当前状态写回磁盘。调用方需持有写锁。
//
// 写失败时**内存里的状态保持不变**是做不到的（已经改过了），所以这里返回错误
// 给上层，由调用方决定要不要提示用户；但绝不回滚内存 —— 那会让「界面显示开了、
// 实际也是开的」和「界面显示开了、其实是关的」两种状态同时存在，更难查。
func (s *switchState) persistLocked() error {
	list := make([]string, 0, len(s.disabled))
	for n := range s.disabled {
		list = append(list, n)
	}
	sort.Strings(list)

	if err := utilsFile.EnsurePrivateDir(dirOf(StatePath)); err != nil {
		return err
	}
	return utilsFile.WriteJsonFile(StatePath, stateFile{Disabled: list})
}

func dirOf(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' || p[i] == '\\' {
			return p[:i]
		}
	}
	return "."
}

// ── 组件的开关实现 ──────────────────────────────────────────────────────────

// LocalSwitch 是组件开关的现成实现，直接嵌进组件类型即可：
//
//	type webhookComponent struct{ component.LocalSwitch }
//
//	func init() {
//	    component.MustRegister(webhookComponent{
//	        LocalSwitch: component.NewLocalSwitch(Name),
//	    })
//	}
//
// # 为什么必须是「本地」的
//
// 它读写的就是状态文件里的那一个位，**不经过注册表**。
// 这一点不是风格问题：注册表的 IsEnabled(name) 会回过头去调用组件的
// Enabled()，如果组件的 Enabled() 又调回注册表，两边就互相调用、
// 直接把栈跑爆（这不是假想，是本项目真的踩到过的一次栈溢出）。
//
// 分工是这样的：
//   - 注册表那一侧（Registry.IsEnabled / SetEnabled）负责校验
//     「组件存不存在、能不能开关」，是给界面和路由闸门用的；
//   - 组件这一侧（本类型）只负责「这个位现在是几」，是给自身的
//     Enablable 接口实现用的。
type LocalSwitch struct{ name string }

// NewLocalSwitch 造一个绑定到该组件名的开关实现。
func NewLocalSwitch(name string) LocalSwitch { return LocalSwitch{name: name} }

// Enabled 见 core/component/state.go 顶部的说明。
func (s LocalSwitch) Enabled() bool { return switches.isEnabled(s.name) }

// SetEnabled 写状态并落盘。
func (s LocalSwitch) SetEnabled(on bool) error { return switches.setEnabled(s.name, on) }

// ── 包级快捷方法 ────────────────────────────────────────────────────────────
//
// 注意：对外的开关判断走的是注册表上的 IsEnabled / SetEnabled（registry.go），
// 它们会顺带校验「这个组件存不存在、能不能开关」。这里只暴露状态文件本身，
// 供需要直接读写落盘状态的场合（测试、导出）使用。

// RecordEnabled 直接记录开关并落盘，不经过注册表校验。
func RecordEnabled(name string, on bool) error { return switches.setEnabled(name, on) }

// DisabledNames 当前被关掉的组件名（有序），界面与日志用。
func DisabledNames() []string { return switches.disabledNames() }
