package backup

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

/**
 * 导入备份。
 *
 * 这是整个备份功能里**唯一会覆盖现有数据**的操作，所以流程必须是：
 *
 *   1. 读清单 → 校验版本、校验包内路径
 *   2. **先给当前 config/ 打一份快照**（不是"先删后写"，是先把旧的整体挪走）
 *   3. 解包覆盖
 *   4. 任何一步出错 → 用快照整体还原
 *
 * 关键是顺序：**快照必须在动任何文件之前**。
 * 如果边解边备份，前几个文件已经覆盖了，回滚就回不干净。
 */

type ImportOptions struct {
	Modules []string `json:"modules"`
	Assets  []string `json:"assets"`
}

type ImportResult struct {
	RestoredModules []string `json:"restoredModules"`
	RestoredAssets  []string `json:"restoredAssets"`
	Skipped         []string `json:"skipped"`
}

// Import 从 zip 内容恢复。返回恢复结果；出错时内部已回滚，调用方直接报错即可。
func Import(raw []byte, opts ImportOptions) (*ImportResult, error) {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, fmt.Errorf("不是有效的备份包（zip）: %w", err)
	}

	// ── ① 清单 ──
	mf, err := readManifest(zr)
	if err != nil {
		return nil, err
	}
	if mf.Version > ManifestVersion {
		return nil, fmt.Errorf("备份包是用更新版本的 Catflap 创建的（清单 v%d），请先升级再导入", mf.Version)
	}

	/**
	 * ② **先把包内所有路径校验一遍，再动磁盘**。
	 *
	 * 全部验完再写，是避免出现"前几个文件已经解压了，
	 * 到第 5 个才发现路径越界"这种**半截状态**。
	 */
	plan, err := planRestore(zr, mf, opts)
	if err != nil {
		return nil, err
	}
	if len(plan.files) == 0 {
		return nil, fmt.Errorf("备份包里没有可恢复的内容（可能都没勾选）")
	}

	// ── ③ 快照（必须在写之前）──
	snapDir, err := takeSnapshot()
	if err != nil {
		return nil, fmt.Errorf("创建恢复前快照失败，已中止：%w", err)
	}

	// ── ④ 执行；出错就回滚 ──
	if err := applyPlan(zr, plan); err != nil {
		if rerr := restoreSnapshot(snapDir); rerr != nil {
			return nil, fmt.Errorf("导入失败（%v），且回滚也失败了（%v）—— 原配置在 %s，请手动恢复", err, rerr, snapDir)
		}
		cleanupSnapshot(snapDir)
		return nil, fmt.Errorf("导入失败，已自动还原到导入前的状态：%w", err)
	}

	// 成功：快照留着一会儿（用户可能马上发现不对），但只保留最近几份
	pruneSnapshots()

	return &ImportResult{
		RestoredModules: plan.modules,
		RestoredAssets:  plan.assets,
		Skipped:         plan.skipped,
	}, nil
}

type restorePlan struct {
	// files: 包内路径 → 磁盘路径
	files   map[string]string
	modules []string
	assets  []string
	skipped []string
}

func planRestore(zr *zip.Reader, mf *Manifest, opts ImportOptions) (*restorePlan, error) {
	wantMod := func(name string) bool {
		if len(opts.Modules) == 0 {
			return true
		}
		for _, w := range opts.Modules {
			if w == name {
				return true
			}
		}
		return false
	}
	wantAsset := func(kind string) bool {
		if len(opts.Assets) == 0 {
			return true
		}
		for _, w := range opts.Assets {
			if w == kind {
				return true
			}
		}
		return false
	}

	plan := &restorePlan{files: map[string]string{}, modules: []string{}, assets: []string{}, skipped: []string{}}

	// 已知模块配置：按清单 + 勾选决定
	known := map[string]string{}
	for _, m := range MODULES {
		known[m.File] = m.Name
	}
	for _, m := range mf.Modules {
		if !m.Exists || !wantMod(m.Name) {
			continue
		}
		// 只认注册表里的路径 —— 包里出现别的 config 文件一律忽略
		if _, ok := known[m.File]; !ok {
			continue
		}
		// zip slip：包内路径必须仍落在 config/ 内
		dir := filepath.Dir(m.File)
		abs, err := safeJoin(dir, filepath.Base(m.File))
		if err != nil {
			return nil, fmt.Errorf("备份包含非法路径: %s", m.File)
		}
		plan.files[m.File] = abs
		plan.modules = append(plan.modules, m.Name)
	}

	// 资产：包内 data/icon/*、data/wallpaper/*
	assetDirs := map[string]bool{}
	for _, a := range ASSETS {
		if !wantAsset(a.Kind) {
			continue
		}
		assetDirs[a.Dir] = true
	}

	restoredAsset := map[string]bool{}
	for _, f := range zr.File {
		name := filepath.ToSlash(f.Name)
		if f.FileInfo().IsDir() {
			continue
		}
		dir := filepath.ToSlash(filepath.Dir(name))
		if !assetDirs[dir] {
			continue
		}
		abs, err := safeJoin(dir, filepath.Base(name))
		if err != nil {
			return nil, fmt.Errorf("备份包含非法路径: %s", name)
		}
		plan.files[name] = abs
		restoredAsset[dir] = true
	}
	for dir := range restoredAsset {
		for _, a := range ASSETS {
			if a.Dir == dir {
				plan.assets = append(plan.assets, a.Kind)
			}
		}
	}

	return plan, nil
}

func applyPlan(zr *zip.Reader, plan *restorePlan) error {
	// 先确保目标目录都在
	for _, abs := range plan.files {
		if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
			return err
		}
	}

	for inZip, abs := range plan.files {
		src, err := zr.Open(inZip)
		if err != nil {
			return fmt.Errorf("读取包内 %s 失败: %w", inZip, err)
		}
		dst, err := os.OpenFile(abs, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		if err != nil {
			src.Close()
			return fmt.Errorf("写入 %s 失败: %w", abs, err)
		}
		_, cerr := io.Copy(dst, src)
		src.Close()
		dst.Close()
		if cerr != nil {
			return fmt.Errorf("写入 %s 失败: %w", abs, cerr)
		}
	}
	return nil
}

func readManifest(zr *zip.Reader) (*Manifest, error) {
	for _, f := range zr.File {
		if filepath.ToSlash(f.Name) != "manifest.json" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		var m Manifest
		if err := json.NewDecoder(rc).Decode(&m); err != nil {
			return nil, fmt.Errorf("备份包里的清单读不出来，可能不是 Catflap 的备份: %w", err)
		}
		if m.Version <= 0 {
			return nil, fmt.Errorf("备份包缺少版本信息，无法安全导入")
		}
		return &m, nil
	}
	return nil, fmt.Errorf("备份包里没有 manifest.json，无法确认内容")
}

/**
 * 快照：把整个 config/ 复制一份到带时间戳的目录。
 *
 * 整目录复制而不是逐个文件复制 —— 逐个复制时"哪些文件已经被覆盖了"
 * 需要额外记账，整目录复制只需一次 copy，回滚也是一次 copy，不会漏。
 */
func takeSnapshot() (string, error) {
	src := "config"
	if _, err := os.Stat(src); err != nil {
		// config 还不存在（全新环境），没什么可快照的
		return "", nil
	}
	name := fmt.Sprintf("config/.snapshot-%s", time.Now().Format("20060102-150405"))
	if err := copyDir(src, name); err != nil {
		return "", err
	}
	return name, nil
}

func restoreSnapshot(snapDir string) error {
	if snapDir == "" {
		return nil
	}
	// 把快照内容拷回 config/（快照目录本身要跳过，否则会套娃）
	return copyDirSkip(snapDir, "config", filepath.Base(snapDir))
}

func cleanupSnapshot(snapDir string) error {
	if snapDir == "" {
		return nil
	}
	return os.RemoveAll(snapDir)
}

// pruneSnapshots 只保留最近 3 份快照，别把磁盘占满
func pruneSnapshots() {
	entries, err := os.ReadDir("config")
	if err != nil {
		return
	}
	var snaps []string
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), ".snapshot-") {
			snaps = append(snaps, e.Name())
		}
	}
	if len(snaps) <= 3 {
		return
	}
	// 名字带时间戳，字典序 = 时间序
	sortStrings(snaps)
	for _, s := range snaps[:len(snaps)-3] {
		os.RemoveAll(filepath.Join("config", s))
	}
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// copyDir 递归复制目录
func copyDir(src string, dst string) error {
	return copyDirSkip(src, dst, "")
}

// copyDirSkip 递归复制，跳过指定名字的条目（防止把快照目录自己套进去）
func copyDirSkip(src string, dst string, skip string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}
	for _, e := range entries {
		name := e.Name()
		if skip != "" && name == skip {
			continue
		}
		s := filepath.Join(src, name)
		d := filepath.Join(dst, name)
		if e.IsDir() {
			if err := copyDirSkip(s, d, skip); err != nil {
				return err
			}
			continue
		}
		if err := copyFile(s, d); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src string, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

/**
 * Peek 只看备份包的清单，不恢复任何东西。
 *
 * 导入是覆盖性操作，让人先看清"这个包里有什么、哪年哪月打的、含不含敏感模块"，
 * 再决定恢复范围 —— 比"传上去直接全覆盖"稳妥得多。
 */
func Peek(raw []byte) (*Manifest, error) {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, fmt.Errorf("不是有效的备份包（zip）: %w", err)
	}
	mf, err := readManifest(zr)
	if err != nil {
		return nil, err
	}
	if mf.Version > ManifestVersion {
		return nil, fmt.Errorf("备份包来自更新版本的 Catflap（清单 v%d），请先升级", mf.Version)
	}
	return mf, nil
}
