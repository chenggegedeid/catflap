package backup

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/chenggegedeid/catflap/core"
)

/**
 * 导出备份。
 *
 * 直接写进内存（bytes.Buffer）再交给响应，不落临时文件 ——
 * 备份是个低频操作，没必要在磁盘上留一份中间产物，
 * 也就不用操心"临时文件没清掉"这类事。
 *
 * 包内容：manifest.json + 勾选的模块配置 + 勾选的资产目录。
 */

// ExportOptions 导出哪些模块/资产。空数组 = 全选
type ExportOptions struct {
	Modules []string `json:"modules"`
	Assets  []string `json:"assets"`
}

type ExportResult struct {
	Data     []byte   `json:"-"`
	Filename string   `json:"filename"`
	Manifest Manifest `json:"manifest"`
}

func Export(opts ExportOptions) (*ExportResult, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	manifest := Manifest{
		Version:    ManifestVersion,
		AppVersion: versionOf(),
		CreatedAt:  time.Now(),
		Modules:    []Module{},
		Assets:     []Asset{},
	}

	// ── 模块配置 ──
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

	for _, m := range MODULES {
		entry := Module{Name: m.Name, Label: m.Label, File: m.File}

		if !wantMod(m.Name) {
			// 不导出也登记进来（exists=false），界面上能显示"这个模块没勾选"
			entry.Exists = false
			manifest.Modules = append(manifest.Modules, entry)
			continue
		}

		abs, err := safeJoin(filepath.Dir(m.File), filepath.Base(m.File))
		if err != nil {
			zw.Close()
			return nil, fmt.Errorf("模块 %s 路径不合法: %w", m.Name, err)
		}
		if _, err := os.Stat(abs); err != nil {
			entry.Exists = false
			manifest.Modules = append(manifest.Modules, entry)
			continue
		}

		n, err := writeFileToZip(zw, abs, m.File)
		if err != nil {
			entry.Exists = false
			manifest.Modules = append(manifest.Modules, entry)
			continue
		}
		entry.Exists = true
		entry.Size = n
		manifest.Modules = append(manifest.Modules, entry)

		if isSensitiveModule(m.Name) {
			manifest.Sensitive = true
		}
	}

	// ── 上传资产 ──
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

	for _, a := range ASSETS {
		asset := Asset{Kind: a.Kind, Dir: a.Dir}
		if !wantAsset(a.Kind) {
			manifest.Assets = append(manifest.Assets, asset)
			continue
		}

		count, total, err := addDirToZip(zw, a.Dir)
		if err != nil {
			// 目录不存在（全新环境很常见）不算错误，记 0 继续
			manifest.Assets = append(manifest.Assets, asset)
			continue
		}
		asset.Count = count
		asset.Bytes = total
		manifest.Assets = append(manifest.Assets, asset)
	}

	// ── 清单（最后写，内容已经齐全）──
	mj, err := marshalManifest(manifest)
	if err != nil {
		zw.Close()
		return nil, err
	}
	mw, err := zw.Create("manifest.json")
	if err != nil {
		zw.Close()
		return nil, err
	}
	if _, err := mw.Write(mj); err != nil {
		zw.Close()
		return nil, err
	}

	if err := zw.Close(); err != nil {
		return nil, err
	}

	sort.Slice(manifest.Modules, func(i, j int) bool { return manifest.Modules[i].Name < manifest.Modules[j].Name })

	return &ExportResult{
		Data:     buf.Bytes(),
		Filename: fmt.Sprintf("catflap-backup-%s.zip", time.Now().Format("20060102-150405")),
		Manifest: manifest,
	}, nil
}

// addDirToZip 把一个目录（只含文件，不递归子目录）打进 zip
func addDirToZip(zw *zip.Writer, dir string) (int, int64, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, 0, err
	}
	count := 0
	var total int64
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		// 只收文件名，杜绝 ../../ 混进来
		abs, err := safeJoin(dir, filepath.Base(e.Name()))
		if err != nil {
			continue
		}
		n, err := writeFileToZip(zw, abs, filepath.ToSlash(filepath.Join(dir, filepath.Base(e.Name()))))
		if err != nil {
			continue
		}
		count++
		total += n
	}
	return count, total, nil
}

// 哪些模块含凭据 —— 备份包里有它们就要警告
func isSensitiveModule(name string) bool {
	switch name {
	case "auth", "ddns", "cert":
		return true
	}
	return false
}

func versionOf() string {
	if core.Version == "" {
		return "dev"
	}
	return core.Version
}
