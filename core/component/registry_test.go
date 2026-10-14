package component

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeComp struct {
	name    string
	initErr error
	inited  bool
	stopped bool
}

func (f *fakeComp) Name() string { return f.name }
func (f *fakeComp) Init(context.Context) error {
	f.inited = true
	return f.initErr
}
func (f *fakeComp) Stop(context.Context) error { f.stopped = true; return nil }

func TestResolveFollowsDependencies(t *testing.T) {
	r := NewRegistry()
	a, b, c := &fakeComp{name: "a"}, &fakeComp{name: "b"}, &fakeComp{name: "c"}

	// 故意按「反序」注册，验证结果由依赖决定而不是注册顺序
	if err := r.Register(c, "b"); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(b, "a"); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(a); err != nil {
		t.Fatal(err)
	}

	got := r.Names()
	want := []string{"a", "b", "c"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("启动顺序应为 %v，实际 %v", want, got)
	}
}

func TestResolveIsStableForIndependentComponents(t *testing.T) {
	r := NewRegistry()
	for _, n := range []string{"x", "y", "z"} {
		if err := r.Register(&fakeComp{name: n}); err != nil {
			t.Fatal(err)
		}
	}
	// 无依赖关系时按注册顺序，保证每次启动顺序一致
	if got := r.Names(); strings.Join(got, ",") != "x,y,z" {
		t.Fatalf("应为 x,y,z，实际 %v", got)
	}
}

func TestRegisterRejectsBadInput(t *testing.T) {
	r := NewRegistry()

	if err := r.Register(nil); err == nil {
		t.Fatal("注册 nil 应当报错")
	}
	if err := r.Register(&fakeComp{name: ""}); err == nil {
		t.Fatal("空名字应当报错")
	}
	if err := r.Register(&fakeComp{name: "a"}, "a"); err == nil {
		t.Fatal("依赖自己应当报错")
	}
	if err := r.Register(&fakeComp{name: "a"}); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(&fakeComp{name: "a"}); err == nil {
		t.Fatal("重复注册应当报错")
	}
	// 重复的依赖应被去重而不是报错
	if err := r.Register(&fakeComp{name: "b"}, "a", "a", " "); err != nil {
		t.Fatal(err)
	}
}

func TestResolveReportsMissingDependency(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(&fakeComp{name: "a"}, "nope"); err != nil {
		t.Fatal(err)
	}
	_, err := r.resolve()
	if err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("应报出缺失的依赖，实际：%v", err)
	}
}

func TestResolveDetectsCycle(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(&fakeComp{name: "a"}, "b"); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(&fakeComp{name: "b"}, "a"); err != nil {
		t.Fatal(err)
	}
	_, err := r.resolve()
	if err == nil || !strings.Contains(err.Error(), "环") {
		t.Fatalf("应报出依赖环，实际：%v", err)
	}
}

func TestInitAllSkipsDependentsOfFailedComponent(t *testing.T) {
	r := NewRegistry()
	failing := &fakeComp{name: "base", initErr: errors.New("boom")}
	dep := &fakeComp{name: "dep"}
	other := &fakeComp{name: "other"}

	if err := r.Register(failing); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(dep, "base"); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(other); err != nil {
		t.Fatal(err)
	}

	err := r.InitAll(context.Background())
	if err == nil {
		t.Fatal("应当返回聚合错误")
	}
	if dep.inited {
		t.Fatal("依赖失败的组件不应被初始化")
	}
	if !other.inited {
		t.Fatal("与失败组件无关的组件仍应初始化")
	}
}

func TestStopAllRunsInReverseOrder(t *testing.T) {
	r := NewRegistry()
	var order []string
	mk := func(name string) Component {
		return &stopRecorder{name: name, sink: &order}
	}
	for _, n := range []string{"a", "b", "c"} {
		if err := r.Register(mk(n)); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.InitAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := r.StopAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if strings.Join(order, ",") != "c,b,a" {
		t.Fatalf("应逆序停止，实际 %v", order)
	}
}

type stopRecorder struct {
	name string
	sink *[]string
}

func (s *stopRecorder) Name() string               { return s.name }
func (s *stopRecorder) Init(context.Context) error { return nil }
func (s *stopRecorder) Stop(context.Context) error { *s.sink = append(*s.sink, s.name); return nil }

func TestSchemaValidateAndRedact(t *testing.T) {
	s := Schema{Fields: []Field{
		{Key: "apiToken", Label: "API Token", Secret: true, Required: true},
		{Key: "zone", Label: "Zone", Type: FieldSelect, Required: true,
			Options: []Option{{Value: "a", Label: "A"}, {Value: "b", Label: "B"}}},
		{Key: "proxy", Label: "走代理", Type: FieldBool},
	}}

	if err := s.Validate(map[string]any{}); err == nil {
		t.Fatal("缺失必填项应当报错")
	}
	if err := s.Validate(map[string]any{"apiToken": "t", "zone": "c"}); err == nil {
		t.Fatal("枚举越界应当报错")
	}
	if err := s.Validate(map[string]any{"apiToken": "t", "zone": "a", "proxy": true}); err != nil {
		t.Fatalf("合法输入不应报错，实际：%v", err)
	}

	out := s.Redact(map[string]any{"apiToken": "secret-value", "zone": "a", "other": 1})
	if out["apiToken"] != "***" {
		t.Fatalf("Secret 字段应被打码，实际 %v", out["apiToken"])
	}
	if out["zone"] != "a" || out["other"] != 1 {
		t.Fatal("非敏感字段不应被改动")
	}
	// 空值保持空，前端才能区分「没配」和「配了」
	if got := s.Redact(map[string]any{"apiToken": ""}); got["apiToken"] != "" {
		t.Fatalf("空值应保持空，实际 %v", got["apiToken"])
	}
}
