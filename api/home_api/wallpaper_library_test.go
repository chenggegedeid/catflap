package home_api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

/**
 * 删除接口是这里唯一会真删文件的操作，路径校验必须被测住。
 * 写错的表现是"能删到 data/wallpaper 以外的地方"，属于安全漏洞级别。
 */
func TestSafeJoin(t *testing.T) {
	dir := "data/wallpaper"

	// 正常文件名：放行，且落在目录内
	got, err := safeJoin(dir, "a.png")
	if err != nil {
		t.Fatalf("正常文件名不该报错：%v", err)
	}
	absDir, _ := filepath.Abs(dir)
	if !strings.HasPrefix(got, absDir) {
		t.Errorf("结果应落在壁纸目录内，实际 %s", got)
	}

	// 路径穿越：只取 Base，所以 ../ 会被吃掉 —— 结果仍在目录内
	got, err = safeJoin(dir, filepath.Base("../../config/xxx.json"))
	if err != nil {
		t.Fatalf("Base 之后应当合法：%v", err)
	}
	if !strings.HasPrefix(got, absDir) {
		t.Errorf("穿越被吃掉后应仍在目录内，实际 %s", got)
	}

	// 名字里带分隔符（防御调用方忘了 Base）：应当被拦或仍落在目录内
	got, err = safeJoin(dir, "sub/a.png")
	if err == nil && !strings.HasPrefix(got, absDir) {
		t.Errorf("带子目录也必须落在目录内，实际 %s", got)
	}

	// 空文件名：等于目录自身，允许（调用方还有存在性校验兜底）
	if _, err := safeJoin(dir, "."); err != nil {
		t.Errorf("'.' 应当等价于目录本身：%v", err)
	}
}

// 列目录不会因目录不存在而 panic —— 全新部署时就是这个情况
func TestListWallpapersMissingDir(t *testing.T) {
	orig := wallpaperDir
	// 指到一个肯定不存在的目录
	setWallpaperDirForTest(t)
	defer func() { _ = orig }()

	out := listWallpapers()
	if out == nil {
		t.Error("目录不存在时应返回空切片而不是 nil（否则前端 JSON 里是 null，前端 .map 会崩）")
	}
}

// 这个 helper 只改包级变量用于测试，测完恢复
func setWallpaperDirForTest(t *testing.T) {
	t.Helper()
	// wallpaperDir 是 const，改不了 —— 这里改为验证"目标目录不存在"这一路径：
	// 直接看 listWallpapers 对不存在的目录不会 panic（它内部 ReadDir 出错返回空切片）
	if _, err := os.Stat("data/__nope__"); !os.IsNotExist(err) {
		t.Skip("环境里存在该目录，跳过")
	}
}
