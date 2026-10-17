package backup

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── 一个最小的合法备份包，用来测导入 ──
func makeZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestManifestRoundTrip(t *testing.T) {
	mf := Manifest{Version: ManifestVersion, AppVersion: "test", Sensitive: true}
	mj, err := marshalManifest(mf)
	if err != nil {
		t.Fatal(err)
	}
	raw := makeZip(t, map[string]string{"manifest.json": string(mj)})

	got, err := Peek(raw)
	if err != nil {
		t.Fatalf("Peek 失败: %v", err)
	}
	if got.Version != ManifestVersion {
		t.Errorf("清单版本应为 %d，实际 %d", ManifestVersion, got.Version)
	}
	if !got.Sensitive {
		t.Error("敏感标记丢了")
	}
}

// 没有清单的包必须拒绝 —— 否则无法确认内容，盲覆盖太危险
func TestPeekRejectsNoManifest(t *testing.T) {
	raw := makeZip(t, map[string]string{"config/homeConfig.json": "{}"})
	if _, err := Peek(raw); err == nil {
		t.Error("没有 manifest.json 的包应当被拒绝")
	}
}

// **zip slip**：包里放 ../../ 路径，必须被拦住，且不能往目录外写文件
func TestImportRejectsZipSlip(t *testing.T) {
	mj, _ := marshalManifest(Manifest{Version: ManifestVersion})
	raw := makeZip(t, map[string]string{
		"manifest.json": string(mj),
		// 恶意路径：想写到 config 外面去
		"config/../../evil.txt": "pwned",
	})

	// 目标文件不应被创建
	evil := filepath.Join("..", "evil.txt")
	defer os.Remove(evil)

	if _, err := Import(raw, ImportOptions{}); err == nil {
		// 即便没报错，也绝不能写出去了
		if _, statErr := os.Stat(evil); statErr == nil {
			t.Fatal("zip slip：文件被写到了目标目录之外")
		}
	}
}

// 正常导入：能恢复，清单里的模块被记出来
func TestImportRestoresModule(t *testing.T) {
	// 用一个临时工作目录，避免污染真实 config
	dir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)

	body := `{"title":"from-backup"}`
	mj, _ := marshalManifest(Manifest{
		Version: ManifestVersion,
		Modules: []Module{{Name: "home", Label: "导航页", File: "config/homeConfig.json", Exists: true}},
	})

	raw := makeZip(t, map[string]string{
		"manifest.json":          string(mj),
		"config/homeConfig.json": body,
	})

	out, err := Import(raw, ImportOptions{})
	if err != nil {
		t.Fatalf("导入失败: %v", err)
	}
	if len(out.RestoredModules) == 0 || out.RestoredModules[0] != "home" {
		t.Errorf("应记录恢复了 home 模块，实际 %v", out.RestoredModules)
	}

	got, err := os.ReadFile("config/homeConfig.json")
	if err != nil {
		t.Fatalf("恢复后的文件读不到: %v", err)
	}
	if strings.TrimSpace(string(got)) != body {
		t.Errorf("内容不符: %q", string(got))
	}
}

// 导入失败时应当自动回滚（用"包内清单指向不存在的文件"制造失败）
func TestImportRollsBackOnFailure(t *testing.T) {
	dir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)

	// 先放一份现有配置
	if err := os.MkdirAll("config", 0755); err != nil {
		t.Fatal(err)
	}
	orig := `{"title":"original"}`
	if err := os.WriteFile("config/homeConfig.json", []byte(orig), 0600); err != nil {
		t.Fatal(err)
	}

	// 清单说有 home，但包里其实没有那个文件 → 解压时会失败
	mj, _ := marshalManifest(Manifest{
		Version: ManifestVersion,
		Modules: []Module{{Name: "home", Label: "导航页", File: "config/homeConfig.json", Exists: true}},
	})
	raw := makeZip(t, map[string]string{"manifest.json": string(mj)})

	if _, err := Import(raw, ImportOptions{}); err == nil {
		t.Fatal("缺少文件的包应当导入失败")
	}

	// 关键：失败后原配置还在
	got, err := os.ReadFile("config/homeConfig.json")
	if err != nil {
		t.Fatalf("回滚失败，原配置丢了: %v", err)
	}
	if strings.TrimSpace(string(got)) != orig {
		t.Errorf("回滚后内容不对: %q", string(got))
	}
}

// 快照目录不会把自己套进去（copyDirSkip 的作用）
func TestSnapshotDoesNotNestItself(t *testing.T) {
	dir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)

	os.MkdirAll("config", 0755)
	os.WriteFile("config/a.json", []byte("{}"), 0600)

	snap, err := takeSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snap == "" {
		t.Fatal("应当生成快照目录")
	}

	// 快照里不该有 .snapshot- 开头的子目录（否则会无限套娃）
	entries, err := os.ReadDir(snap)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".snapshot-") {
			t.Errorf("快照把自己套进去了: %s", e.Name())
		}
	}
	fmt.Fprintln(os.Stderr, "快照目录:", snap)
}
