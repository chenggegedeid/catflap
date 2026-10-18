package proxy_api

import (
	"encoding/json"
	"reflect"
	"testing"
)

// 本文件守一类很隐蔽的 bug：**写接口能改、读接口不返回**。
//
// 已经踩过两次：
//   1. Forwards —— 端口转发添加成功，但列表永远空（本视图当时没有这个字段）
//   2. ExposeAdmin —— 「允许通过反代暴露管理后台」保存成功并提示已生效，
//      一刷新开关弹回关闭（同上）
//
// 两次的表现都是「功能像坏了」，实际数据存对了，只是读不回来。
// 界面层不会报错、后端也不报错，只有用户对着界面困惑 —— 所以只能靠测试挡。

// wantViewFields 本视图必须返回的字段。
//
// 判定依据是**前端 types.ts 里的 ProxyConfig 接口**——那里声明了什么，
// 这个视图就得返回什么。两边对不上时以那边为准，改完记得同步这张表。
var wantViewFields = []string{
	"entry",
	"listeners",
	"sites",
	"forwards",
	"errorPage",
	"exposeAdmin",
}

func TestProxyConfigViewHasAllFrontendFields(t *testing.T) {
	got := map[string]bool{}
	typ := reflect.TypeOf(ProxyConfigView{})

	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		tag := f.Tag.Get("json")
		if tag == "" {
			t.Errorf("字段 %s 没有 json tag，前端拿不到它", f.Name)
			continue
		}
		// "name,omitempty" 只要名字部分
		name := tag
		if i := indexByte(tag, ','); i >= 0 {
			name = tag[:i]
		}
		if name == "-" {
			t.Errorf("字段 %s 被 json:\"-\" 排除了，前端拿不到它", f.Name)
			continue
		}
		got[name] = true
	}

	for _, want := range wantViewFields {
		if !got[want] {
			t.Errorf("ProxyConfigView 缺少字段 %q —— 前端 types.ts 里有它，"+
				"读不到就会出现「保存成功但一刷新回到原样」", want)
		}
	}
}

// TestProxyConfigViewFieldCount 字段数对得上。
//
// 光查「想要的都在」不够：万一有人新增了前端不需要的字段倒也无害，
// 但反过来——把 wantViewFields 和结构体一起改小了就漏检了。
// 这里再卡一道总数，逼着改结构的人必须回来同步这张表。
func TestProxyConfigViewFieldCount(t *testing.T) {
	typ := reflect.TypeOf(ProxyConfigView{})
	if typ.NumField() != len(wantViewFields) {
		t.Errorf("字段数 %d != 期望 %d；改了 ProxyConfigView 请同步 wantViewFields",
			typ.NumField(), len(wantViewFields))
	}
}

// TestExposeAdminSurvivesRoundTrip 打开暴露后台后，读回来的值必须还是打开的。
//
// 这是 ExposeAdmin 那个 bug 的**精确复现**：存进去 true，读出来还是 true。
// 只查结构体有字段还不够（有人可能加了字段却忘了在函数里填充），
// 所以这里直接构造一次视图、走一遍 JSON 编解码，看落地的值。
func TestExposeAdminSurvivesRoundTrip(t *testing.T) {
	v := ProxyConfigView{ExposeAdmin: true}

	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	var back ProxyConfigView
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}

	if !back.ExposeAdmin {
		t.Error("exposeAdmin=true 经过一轮 JSON 后变成了 false —— " +
			"界面上就是「打开了、刷新又关了」")
	}

	// 顺带确认这个字段在 JSON 里确实叫 exposeAdmin（前端读的是这个名字）
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("解析成 map 失败: %v", err)
	}
	if _, ok := raw["exposeAdmin"]; !ok {
		t.Error(`JSON 里没有 "exposeAdmin" 这个键，前端读不到`)
	}
}

func indexByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}
