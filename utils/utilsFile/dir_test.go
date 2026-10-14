package utilsFile

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// 权限语义只在类 Unix 上成立：Windows 的 os.Chmod 只能切只读位，
// 0755 和 0700 在那套模型下测不出差别。真正的权限验收放在 Linux CI 上跑，
// 这里把带权限断言的用例在 Windows 上跳过，逻辑用例照常跑。
const windowsSkipReason = "Windows 的 os.Chmod 只影响只读位，权限位测不出差别；由 Linux CI 覆盖"

func permOf(t *testing.T, path string) fs.FileMode {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return fi.Mode().Perm()
}

func TestEnsurePrivateDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "config")
	if err := EnsurePrivateDir(dir); err != nil {
		t.Fatalf("创建私有目录失败：%v", err)
	}
	// 幂等：第二次调用时目录已存在，不该出错
	if err := EnsurePrivateDir(dir); err != nil {
		t.Fatalf("重复调用应无错：%v", err)
	}
	if runtime.GOOS == "windows" {
		t.Skip(windowsSkipReason)
	}
	if got := permOf(t, dir); got != PrivateDirMode {
		t.Fatalf("目录权限 = %o，期望 %o", got, PrivateDirMode)
	}
}

// 关键回归：老版本用 0755/0644 建的目录和文件，SecureTree 要能就地纠正过来。
// 这正是升级用户遇到的情况——不纠正的话，历史配置会一直敞着。
func TestSecureTreeFixesLoosePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip(windowsSkipReason)
	}
	root := t.TempDir()
	// 模拟历史遗留：宽权限的目录树，含嵌套的证书私钥目录
	nested := filepath.Join(root, "config", "cert", "1")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	loose := filepath.Join(root, "config", "authConfig.json")
	if err := os.WriteFile(loose, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	key := filepath.Join(nested, "privkey.pem")
	if err := os.WriteFile(key, []byte("KEY"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := SecureTree(filepath.Join(root, "config")); err != nil {
		t.Fatalf("SecureTree 失败：%v", err)
	}

	for _, d := range []string{
		filepath.Join(root, "config"),
		filepath.Join(root, "config", "cert"),
		nested,
	} {
		if got := permOf(t, d); got != PrivateDirMode {
			t.Errorf("目录 %s 权限 = %o，期望 %o", d, got, PrivateDirMode)
		}
	}
	for _, f := range []string{loose, key} {
		if got := permOf(t, f); got != PrivateFileMode {
			t.Errorf("文件 %s 权限 = %o，期望 %o", f, got, PrivateFileMode)
		}
	}
}

// 全新机器上配置目录还不存在，这是正常状态，不该当成错误
func TestSecureTreeMissingRootIsFine(t *testing.T) {
	if err := SecureTree(filepath.Join(t.TempDir(), "not-created-yet")); err != nil {
		t.Fatalf("目录不存在应返回 nil，实际：%v", err)
	}
}

// WriteJsonFile 对已存在的宽权限文件也要收回 0600——
// os.WriteFile 只在新建时应用权限，靠它自己纠正不了老文件
func TestWriteJsonFileTightensExistingFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip(windowsSkipReason)
	}
	p := filepath.Join(t.TempDir(), "c.json")
	if err := os.WriteFile(p, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteJsonFile(p, map[string]string{"a": "b"}); err != nil {
		t.Fatal(err)
	}
	if got := permOf(t, p); got != PrivateFileMode {
		t.Fatalf("权限 = %o，期望 %o", got, PrivateFileMode)
	}
}

func TestWriteJsonFileCreatesPrivateFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip(windowsSkipReason)
	}
	p := filepath.Join(t.TempDir(), "new.json")
	if err := WriteJsonFile(p, map[string]int{"n": 1}); err != nil {
		t.Fatal(err)
	}
	if got := permOf(t, p); got != PrivateFileMode {
		t.Fatalf("权限 = %o，期望 %o", got, PrivateFileMode)
	}
	// 内容仍要能正常读回
	got, err := ReadJsonFile[map[string]int](p)
	if err != nil {
		t.Fatalf("读回失败：%v", err)
	}
	if got["n"] != 1 {
		t.Fatalf("读回内容 = %v", got)
	}
}
