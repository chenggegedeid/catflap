package system_api

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/chenggegedeid/catflap/utils/res"

	"github.com/gin-gonic/gin"
)

// logRoot 和 core.InitLogger 里 Myhook 的 logPath 一致，按天分目录
const logRoot = "logs"

// tailLimit 从文件尾部往回读多少字节。一天的 info.log 能到好几 MB，
// 整个读进内存再切没必要，够翻出一页就行。
const tailLimit = 2 << 20 // 2MB

// ansiRe 日志落盘时带着终端颜色码（写的时候是给控制台看的），送进浏览器前要剥掉
var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// lineRe 一行的形状。三种都要认：
//
// 新格式（带模块）：
//
//	[2026-10-15 17:20:31] [info] [stun] [scheduler.go:114] 正文
//
// 旧格式（有 caller、无模块，升级前的文件）：
//
//	[2026-09-18 00:00:01] [info] [network.go:73,github.com/...] 正文
//
// 旧格式（SetReportCaller(false)、两者都没有）：
//
//	[2026-09-18 10:00:00] [warn] 正文
//
// **三种都要认**：升级之后用户还想翻升级前的日志，只认新格式会让老文件
// 整片解析失败（显示成一堆无格式的裸文本）。
var lineRe = regexp.MustCompile(`^\[(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2})\] \[(\w+)\](?: \[([^\]]*)\])?(?: \[([^\]]*)\])?\s*(.*)$`)

// moduleRe 模块名形状：只含小写字母。用来区分"第三段是模块名还是 caller"。
var moduleRe = regexp.MustCompile(`^[a-z]+$`)

// dayRe 日期目录名。同时兼作入参白名单——这个值会拼进文件路径，
// 不校验就等于把 logs/../../ 这种路径交给调用方随便写。
var dayRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

type LogEntry struct {
	Time  string `json:"time"`
	Level string `json:"level"`
	// Module 这条日志属于哪个模块（stun / proxy / ddns ...）。
	// 由 core 从调用栈推出来写进日志行，这里读出来给前端做分类。
	// 升级前的老日志没有这一段，会是空字符串 → 前端归到「其它」。
	Module  string `json:"module"`
	Caller  string `json:"caller"`
	Message string `json:"message"`
}

// LogDaysView 有哪几天的日志，新的在前
func (SystemApi) LogDaysView(c *gin.Context) {
	res.OkWithData(logDays(), c)
}

// logDays 有哪几天的日志，新的在前。列表接口和分类统计都要用，抽出来共用。
func logDays() []string {
	items, err := os.ReadDir(logRoot)
	if err != nil {
		// 还没写过日志时目录不存在，这不是错误
		return []string{}
	}

	days := make([]string, 0, len(items))
	for _, it := range items {
		if it.IsDir() && dayRe.MatchString(it.Name()) {
			days = append(days, it.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(days)))
	return days
}

// LogListView 读某一天的日志。按 level / 关键字过滤，返回最新的 limit 条，新的在前。
func (SystemApi) LogListView(c *gin.Context) {
	day := c.Query("day")
	if day == "" {
		res.FailWithMsg("请指定日期", c)
		return
	}
	if !dayRe.MatchString(day) {
		res.FailWithMsg("日期格式不对", c)
		return
	}

	// 只有这两个文件，写死成白名单；err.log 是 info.log 里错误级别的副本
	name := "info.log"
	if c.Query("file") == "err" {
		name = "err.log"
	}

	limit, _ := strconv.Atoi(c.Query("limit"))
	if limit <= 0 {
		limit = 200
	}
	if limit > 2000 {
		limit = 2000
	}

	level := strings.ToLower(strings.TrimSpace(c.Query("level")))
	keyword := strings.ToLower(strings.TrimSpace(c.Query("keyword")))

	raw, err := readTail(filepath.Join(logRoot, day, name), tailLimit)
	if err != nil {
		if os.IsNotExist(err) {
			res.OkWithData(gin.H{"list": []LogEntry{}, "truncated": false}, c)
			return
		}
		res.FailWithMsg("读取日志失败："+err.Error(), c)
		return
	}

	entries := parseLog(raw)

	// 从最新一条往回收集，够 limit 条就停，不用把一整天都过一遍
	out := make([]LogEntry, 0, limit)
	for i := len(entries) - 1; i >= 0 && len(out) < limit; i-- {
		e := entries[i]
		if level != "" && e.Level != level {
			continue
		}
		if keyword != "" && !strings.Contains(strings.ToLower(e.Message), keyword) &&
			!strings.Contains(strings.ToLower(e.Caller), keyword) {
			continue
		}
		out = append(out, e)
	}

	res.OkWithData(gin.H{
		"list": out,
		// 只读了尾部一段，前面还有没读到的，页面上要讲清楚免得用户以为日志就这些
		"truncated": len(raw) >= tailLimit,
	}, c)
}

// readTail 读文件末尾最多 max 字节。文件比 max 小就整个读。
func readTail(path string, max int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return nil, err
	}

	size := st.Size()
	offset := int64(0)
	if size > max {
		offset = size - max
		size = max
	}

	buf := make([]byte, size)
	if _, err := f.ReadAt(buf, offset); err != nil && len(buf) == 0 {
		return nil, err
	}

	// 从中间切进来的话第一行多半是半截，丢掉
	if offset > 0 {
		if i := strings.IndexByte(string(buf), '\n'); i >= 0 {
			buf = buf[i+1:]
		}
	}
	return buf, nil
}

// parseLog 把日志正文切成一条条。对不上格式的行当成上一条的续行
// （日志正文里带换行时会这样），没有上一条就原样留着，不丢内容。
func parseLog(raw []byte) []LogEntry {
	lines := strings.Split(ansiRe.ReplaceAllString(string(raw), ""), "\n")
	entries := make([]LogEntry, 0, len(lines))

	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}

		m := lineRe.FindStringSubmatch(line)
		if m == nil {
			if n := len(entries); n > 0 {
				entries[n-1].Message += "\n" + line
			} else {
				entries = append(entries, LogEntry{Message: line})
			}
			continue
		}

		// 三种形态，靠"第几段有内容"区分：
		//   [时间] [级别] [模块] [caller] 正文   ← 新
		//   [时间] [级别] [caller] 正文          ← 旧（有 caller）
		//   [时间] [级别] 正文                   ← 旧（没 caller）
		module, caller, message := "", "", ""
		seg3, seg4 := m[3], m[4]
		if seg3 == "" {
			// 只有两段：正文在 m[5]（正则里前面的可选组都空匹配了）
			message = strings.TrimSpace(m[5])
		} else if moduleRe.MatchString(seg3) && seg4 != "" {
			// 三段都有，且第三段是纯小写字母 → 那是模块名
			module, caller = seg3, seg4
			message = strings.TrimSpace(m[5])
		} else if moduleRe.MatchString(seg3) && seg4 == "" {
			// 第三段是纯小写字母但没有第四段：这是新格式的模块名，
			// 正文被正则吃进了 m[5]？不，这种情况正文在 m[5] 里
			// 更可能是老日志里 caller 恰好是个纯单词（极少），
			// 保守起见按 caller 处理，保证正文不丢
			caller = seg3
			message = strings.TrimSpace(m[5])
		} else {
			// 第三段不是纯小写字母 → 老格式的 caller
			caller = seg3
			if seg4 != "" {
				// 理论上不该出现（老格式只有一段），拼回正文以免丢内容
				message = strings.TrimSpace(seg4 + " " + m[5])
			} else {
				message = strings.TrimSpace(m[5])
			}
		}

		entries = append(entries, LogEntry{
			Time:    m[1],
			Level:   strings.ToLower(m[2]),
			Module:  module,
			Caller:  caller,
			Message: message,
		})
	}
	return entries
}

// ===================== 分类统计（折叠态用）=====================

// ModuleStat 一个模块的日志条数与异常数
type ModuleStat struct {
	Module string `json:"module"`
	Total  int    `json:"total"`
	Warn   int    `json:"warn"`
	Error  int    `json:"error"`
	// LastAt 该模块最后一条日志的时间，界面上给个"最近活动"的感知
	LastAt string `json:"lastAt"`
}

// LogSummaryResponse 分类摘要 + 总计
type LogSummaryResponse struct {
	Modules []ModuleStat `json:"modules"`
	Total   int          `json:"total"`
	Warn    int          `json:"warn"`
	Error   int          `json:"error"`
	// Truncated 日志超过读取上限时为 true（界面提示"只统计了最近 N 条"）
	Truncated bool `json:"truncated"`
}

// LogSummaryView 按模块统计当天日志，**只回数字不回内容**。
//
// 为什么单开一个接口而不是让前端拉全量再自己数：折叠态只是要几个数字，
// 让前端为了显示"穿透 1204 条"去下载 2MB 日志，打开页面就卡一下。
// 这里扫一遍文件（当天日志通常几 MB，几百毫秒）只回几百字节。
func (SystemApi) LogSummaryView(c *gin.Context) {
	day := c.Query("day")
	if day == "" {
		days := logDays()
		if len(days) > 0 {
			day = days[0] // 默认最新一天
		}
	}
	if !dayRe.MatchString(day) {
		res.FailWithMsg("日期格式不对", c)
		return
	}

	name := "info.log"
	if c.Query("file") == "err" {
		name = "err.log"
	}

	raw, err := readTail(filepath.Join(logRoot, day, name), tailLimit)
	if err != nil {
		// 那天没有日志不是错误，回空统计
		res.OkWithData(LogSummaryResponse{Modules: []ModuleStat{}}, c)
		return
	}
	truncated := int64(len(raw)) >= tailLimit

	entries := parseLog(raw)
	byModule := map[string]*ModuleStat{}
	var total, warn, errCount int

	for _, e := range entries {
		key := e.Module
		if key == "" {
			// 升级前的老日志没有模块段，归到 other（而不是丢掉）
			key = "other"
		}
		st := byModule[key]
		if st == nil {
			st = &ModuleStat{Module: key}
			byModule[key] = st
		}
		st.Total++
		st.LastAt = e.Time
		switch e.Level {
		case "warn", "warning":
			st.Warn++
			warn++
		case "error", "fatal", "panic":
			st.Error++
			errCount++
		}
		total++
	}

	out := make([]ModuleStat, 0, len(byModule))
	for _, st := range byModule {
		out = append(out, *st)
	}
	// 排序：**有异常的排前面**，其次按条数。
	// 用户打开日志页的动机通常是"哪儿出问题了"，不是"哪个模块话多"。
	sort.Slice(out, func(i, j int) bool {
		ei, ej := out[i].Warn+out[i].Error, out[j].Warn+out[j].Error
		if (ei > 0) != (ej > 0) {
			return ei > 0
		}
		if ei != ej {
			return ei > ej
		}
		return out[i].Total > out[j].Total
	})

	res.OkWithData(LogSummaryResponse{
		Modules:   out,
		Total:     total,
		Warn:      warn,
		Error:     errCount,
		Truncated: truncated,
	}, c)
}
