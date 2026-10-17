package audit

import (
	"os"
	"path/filepath"
	"testing"
)

// TestReadDaySkipsBadLines 一行坏数据不该让整天的记录都读不出来。
//
// # 这条守的是一个真实踩到的坑
//
// 以前 ReadDay 用 json.Decoder 循环，遇到解码错误就 break。当时想的是
// 「文件是自己写的，怎么会坏」—— 结果实测遇到：某一行 time 少了时区，
// decoder 从那一行开始停住，**后面所有记录静默丢失**。
//
// 表现是审计页只显示一半数据，没有任何报错。这种问题几乎不可能从现象
// 反推原因，所以必须钉住。
func TestReadDaySkipsBadLines(t *testing.T) {
	dir := t.TempDir()
	setAuditDir(t, dir)

	const day = "2026-10-17"
	lines := []string{
		`{"time":"2026-10-17T10:00:00+08:00","ip":"1.1.1.1","path":"/a","status":200}`,
		`{"time":"2026-10-17T10:01:00+08:00","ip":"8.8.8.8","path":"/b","status":404}`,
		// 坏行：time 没有时区，Go 解析 time.Time 会失败
		`{"time":"2026-10-17T10:02:00","ip":"9.9.9.9","path":"/bad","status":404}`,
		// 坏行：根本不是 JSON
		`这里不是 json`,
		// 坏行后的正常行：**必须仍然能读到**，这是这条测试的全部意义
		`{"time":"2026-10-17T10:03:00+08:00","ip":"2001:4860:4860::8888","path":"/c","status":404}`,
		`{"time":"2026-10-17T10:04:00+08:00","ip":"114.114.114.114","path":"/d","status":200}`,
	}
	f, err := os.Create(filepath.Join(dir, day+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range lines {
		if _, err := f.WriteString(l + "\n"); err != nil {
			t.Fatal(err)
		}
	}
	_ = f.Close()

	recs, err := ReadDay(day, "", 0)
	if err != nil {
		t.Fatalf("ReadDay 返回错误：%v", err)
	}

	// 4 条好数据（坏行后面的两条也在内）
	if len(recs) != 4 {
		ips := make([]string, 0, len(recs))
		for _, r := range recs {
			ips = append(ips, r.IP)
		}
		t.Fatalf("读到 %d 条（%v），期望 4 条 —— 坏行不该截断后面的记录", len(recs), ips)
	}

	// 坏行之后的那两条要真的在
	want := map[string]bool{"2001:4860:4860::8888": false, "114.114.114.114": false}
	for _, r := range recs {
		if _, ok := want[r.IP]; ok {
			want[r.IP] = true
		}
	}
	for ip, found := range want {
		if !found {
			t.Errorf("坏行之后的记录 %s 没被读到", ip)
		}
	}
}

// TestReadDayFiltersByIP 按 IP 过滤。
func TestReadDayFiltersByIP(t *testing.T) {
	dir := t.TempDir()
	setAuditDir(t, dir)

	day := "2026-10-17"
	f, _ := os.Create(filepath.Join(dir, day+".jsonl"))
	f.WriteString(`{"time":"2026-10-17T10:00:00+08:00","ip":"1.1.1.1","path":"/a"}` + "\n")
	f.WriteString(`{"time":"2026-10-17T10:01:00+08:00","ip":"2.2.2.2","path":"/b"}` + "\n")
	f.Close()

	recs, err := ReadDay(day, "2.2.2.2", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 || recs[0].IP != "2.2.2.2" {
		t.Errorf("按 IP 过滤结果不对：%+v", recs)
	}
}

// setAuditDir 把明细目录指到临时目录，并在用例结束时还原。
func setAuditDir(t *testing.T, dir string) {
	t.Helper()
	old := auditDir
	auditDir = dir
	t.Cleanup(func() { auditDir = old })
}
