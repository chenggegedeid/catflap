package utilsFile

import (
	"io/fs"
	"os"
	"path/filepath"
)

// 敏感内容的权限约定：
//   - config/ 里有 JWT 签名密钥、Cloudflare Token、DDNS 密钥、Webhook 密钥；
//   - data/cert/{id}/ 里有站点私钥。
//
// 这些内容不能让同机其它用户读到，否则对方可以伪造登录态、改掉 DNS 记录、
// 或者伪装成你的站点。所以目录只许属主进入（0700），文件只许属主读写（0600）。
//
// 注意：Windows 上 os.Chmod 只影响只读位，这两个值实际不生效；Windows 用户目录
// 默认已有 ACL 隔离，风险远小于多用户 Linux/NAS。本约束主要面向 Linux 部署。
const (
	PrivateDirMode  fs.FileMode = 0o700
	PrivateFileMode fs.FileMode = 0o600
)

// EnsurePrivateDir 确保目录存在，且只有属主能访问。
// os.MkdirAll 对已存在的目录不会改权限，所以这里额外 Chmod 一次：
// 老版本用 0755 建的目录，升级后第一次启动就能被纠正过来。
func EnsurePrivateDir(dir string) error {
	if err := os.MkdirAll(dir, PrivateDirMode); err != nil {
		return err
	}
	return os.Chmod(dir, PrivateDirMode)
}

// SecureTree 把 root 及其下所有子目录收成 0700、所有普通文件收成 0600，
// 用于纠正历史版本留下的宽权限。root 不存在时视为「还没建」，直接返回 nil。
//
// 只用于程序自己管理的敏感目录（config/、data/cert/），不要指向用户资源目录
// （data/icon、data/wallpaper 是要通过 HTTP 公开出去的，不该照这个来）。
func SecureTree(root string) error {
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil
	}
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.Chmod(p, PrivateDirMode)
		}
		return os.Chmod(p, PrivateFileMode)
	})
}
