package core

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"strings"
	"sync"

	"github.com/chenggegedeid/catflap/utils/utilsFile"

	"github.com/sirupsen/logrus"
)

type MyLog struct {
}

// 颜色
const (
	red    = 31
	yellow = 33
	blue   = 36
	gray   = 37
)

// Format 日志格式。
//
// **终端上色，文件里不上色**：颜色是 ANSI 转义序列（\x1b[36m 这种），
// 写进文件之后 cat / grep / 编辑器里全是乱码，而日志文件正是要拿来查的。
// 判断依据是这次输出到哪：终端就带颜色，重定向到文件就去掉。
//
// 这里通过 entry.Logger.Out 是否等于 os.Stdout 来判断 —— logrus 的
// 文件落盘走的是 Hook（Fire 里 write），不经过 Formatter 的输出目标，
// 所以 Hook 写文件时会另走 textFormatter，天然没有颜色。
func (MyLog) Format(entry *logrus.Entry) ([]byte, error) {
	var levelColor int
	switch entry.Level {
	case logrus.DebugLevel, logrus.TraceLevel:
		levelColor = gray
	case logrus.WarnLevel:
		levelColor = yellow
	case logrus.ErrorLevel, logrus.FatalLevel, logrus.PanicLevel:
		levelColor = red
	default:
		levelColor = blue
	}

	// 落文件的那条路径（Hook）用无颜色的格式，见 fileFormatter
	if _, isFile := entry.Logger.Out.(*os.File); !isFile || entry.Logger.Out != os.Stdout {
		return fileFormat(entry)
	}

	var b *bytes.Buffer
	if entry.Buffer != nil {
		b = entry.Buffer
	} else {
		b = &bytes.Buffer{}
	}
	timestamp := entry.Time.Format("2006-01-02 15:04:05")
	mod := ModuleFromCaller(entry)
	if entry.HasCaller() {
		funcVal := entry.Caller.Function
		fileVal := fmt.Sprintf("%s:%d", path.Base(entry.Caller.File), entry.Caller.Line)
		fmt.Fprintf(b, "[%s] \x1b[%dm[%s]\x1b[0m \x1b[%dm[%s]\x1b[0m [%s,%s] \x1b[%dm %s \x1b[0m\n", timestamp, levelColor, entry.Level, levelColor, mod, fileVal, funcVal, levelColor, entry.Message)
	} else {
		fmt.Fprintf(b, "[%s] \x1b[%dm[%s]\x1b[0m \x1b[%dm[%s]\x1b[0m \x1b[%dm %s \x1b[0m\n", timestamp, levelColor, entry.Level, levelColor, mod, levelColor, entry.Message)
	}
	return b.Bytes(), nil
}

// fileFormatter 写文件用的格式：不带任何 ANSI 转义，纯文本可直接 grep。
type fileFormatter struct{}

func (fileFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	return fileFormat(entry)
}

// ModuleFromCaller 从调用栈推出这条日志属于哪个模块。
//
// 不手工标是因为那要改几百处 logrus.Info 调用，而且新代码一定会漏标。
// 从 Caller 推是零成本的，且天然不会漏 —— 每条日志都有 caller。
//
// 路径形态：
//
//	.../modules/stun/scheduler.go      → stun
//	.../api/stun_api/stun_service.go   → stun（api 层归一回模块名）
//	.../core/component/registry.go     → core
//	.../routers/proxy_routers.go       → router
//	.../middleware/auth.go             → core（中间件算框架层）
func ModuleFromCaller(entry *logrus.Entry) string {
	// 不用 entry.HasCaller()：它要求 Logger、Caller、Caller.File 都齐，
	// 而这里只需要**函数名**。用带点号的判断更直接，也不依赖 logrus
	// 内部的"什么算有 caller"约定（调试时被这个约定坑了一轮：
	// 构造的 entry 只缺 File 就一直被判成没有 caller）。
	if entry == nil || entry.Caller == nil || entry.Caller.Function == "" {
		return "other"
	}
	pkg := entry.Caller.Function

	// 按 package 路径找，而不是文件名 —— 文件名在 modules/stun/model/
	// 这种子包里推不出模块
	if i := strings.Index(pkg, "/modules/"); i >= 0 {
		rest := pkg[i+len("/modules/"):]
		// 斜杠（文件路径）或点号（函数名）都作为分隔
		end := len(rest)
		if j := strings.IndexAny(rest, "/."); j >= 0 {
			end = j
		}
		return rest[:end]
	}
	// api 层：api/stun_api.XXX 或 api/stun_api/xxx.go 两种形态都收，
	// 统一去掉 _api 后缀归到模块名（api 层的日志和模块层放一起才好看）
	if i := strings.Index(pkg, "/api/"); i >= 0 {
		rest := pkg[i+len("/api/"):]
		// 截到第一个分隔符：斜杠（文件路径）或点号（函数名）
		end := len(rest)
		if j := strings.IndexAny(rest, "/."); j >= 0 {
			end = j
		}
		return strings.TrimSuffix(rest[:end], "_api")
	}
	// 注意 Caller.Function 是**完整函数名**，形如
	// "github.com/x/catflap/core.ModuleFromCaller" —— 包名后面直接跟
	// `.函数名`，**没有斜杠**。所以 "/core/" 这种写法永远匹配不上，
	// 单测就是这么抓出来的。
	//
	// 判断方式：找到 "/<包名>"，再看它后面是 "." / "/" / 结尾，
	// 避免 core 误匹配到 corexyz 这种前缀相同的包。
	if hasPkg(pkg, "routers") {
		return "router"
	}
	if hasPkg(pkg, "core") || hasPkg(pkg, "middleware") {
		return "core"
	}
	return "other"
}

// hasPkg 函数路径里的包名是不是指定的那个。
func hasPkg(fn, name string) bool {
	i := strings.Index(fn, "/"+name)
	if i < 0 {
		return false
	}
	rest := fn[i+len(name)+1:]
	if rest == "" {
		return true
	}
	return rest[0] == '.' || rest[0] == '/'
}

func fileFormat(entry *logrus.Entry) ([]byte, error) {
	var b *bytes.Buffer
	if entry.Buffer != nil {
		b = entry.Buffer
	} else {
		b = &bytes.Buffer{}
	}
	ts := entry.Time.Format("2006-01-02 15:04:05")
	mod := ModuleFromCaller(entry)
	if entry.HasCaller() {
		fmt.Fprintf(b, "[%s] [%s] [%s] [%s:%d] %s\n",
			ts, entry.Level, mod, path.Base(entry.Caller.File), entry.Caller.Line, entry.Message)
	} else {
		fmt.Fprintf(b, "[%s] [%s] [%s] %s\n", ts, entry.Level, mod, entry.Message)
	}
	return b.Bytes(), nil
}

func InitLogger() {
	logrus.SetLevel(logrus.DebugLevel)
	logrus.SetReportCaller(true)
	logrus.SetFormatter(MyLog{})
	logrus.AddHook(&Myhook{
		logPath: "logs",
	})
}

type Myhook struct {
	file    *os.File
	errFile *os.File
	date    string
	logPath string
	mu      sync.Mutex
}

func (hook *Myhook) Fire(entry *logrus.Entry) error {
	hook.mu.Lock()
	defer hook.mu.Unlock()

	date := entry.Time.Format("2006-01-02")
	if hook.date != date {
		if err := hook.rotateFiles(date); err != nil {
			return fmt.Errorf("rotateFiles失败: %w", err)
		}
		hook.date = date
	}

	// 用无颜色格式：文件是拿来查的，ANSI 转义码在里面只是乱码
	msgBytes, err := fileFormat(entry)
	if err != nil {
		return fmt.Errorf("序列化日志失败: %w", err)
	}
	msg := string(msgBytes)

	if hook.file != nil {
		if _, err := hook.file.Write([]byte(msg)); err != nil {
			return fmt.Errorf("写入info.log失败: %w", err)
		}
	}

	if entry.Level <= logrus.ErrorLevel && hook.errFile != nil {
		if _, err := hook.errFile.Write([]byte(msg)); err != nil {
			return fmt.Errorf("写入err.log失败: %w", err)
		}
	}

	return nil
}

func (hook *Myhook) rotateFiles(date string) error {
	// 关闭旧文件
	if hook.file != nil {
		hook.file.Close()
		hook.file = nil
	}
	if hook.errFile != nil {
		hook.errFile.Close()
		hook.errFile = nil
	}

	// 日志里会出现内网 IP、访问路径、报错详情，不该让同机其它用户随便看，
	// 所以按敏感目录处理：目录 0700、文件 0600（同 config/ 一个标准）
	logDir := fmt.Sprintf("%s/%s", hook.logPath, date)
	if err := utilsFile.EnsurePrivateDir(logDir); err != nil {
		return fmt.Errorf("创建日志目录失败 %s: %w", logDir, err)
	}

	// 打开 info.log
	infoPath := fmt.Sprintf("%s/info.log", logDir)
	file, err := os.OpenFile(infoPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, utilsFile.PrivateFileMode)
	if err != nil {
		return fmt.Errorf("打开info.log失败 %s: %w", infoPath, err)
	}
	hook.file = file

	// 打开 err.log
	errPath := fmt.Sprintf("%s/err.log", logDir)
	errFile, err := os.OpenFile(errPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, utilsFile.PrivateFileMode)
	if err != nil {
		// 打开 err.log 失败时，关掉已打开的 info.log，避免 fd 泄漏
		hook.file.Close()
		hook.file = nil
		return fmt.Errorf("打开err.log失败 %s: %w", errPath, err)
	}
	hook.errFile = errFile

	return nil
}

// Levels 决定哪些级别日志走 Fire 方法
func (*Myhook) Levels() []logrus.Level {
	return logrus.AllLevels
}
