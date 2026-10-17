package backup

import (
	"encoding/json"
	"time"
)

/**
 * 备份清单。
 *
 * 为什么要有清单，而不是"把 config/ 整个打包"：
 *
 * 1. **能校验再恢复**：打开一个备份包，得先知道它是什么版本打的、
 *    含哪些模块，再决定能不能导进来。没有清单就只能盲覆盖。
 * 2. **能选模块**：有人只想迁导航页，不想把 DNS token 带到新机器上。
 * 3. **能向后兼容**：将来配置结构变了（schemaVersion 升到 4），
 *    老备份包靠清单里的版本信息决定"直接恢复"还是"先跑迁移"。
 */

// ManifestVersion 清单格式本身的版本（不是各模块配置的 schemaVersion）
const ManifestVersion = 1

type Manifest struct {
	// Version 清单格式版本。将来清单结构变了靠它区分
	Version int `json:"version"`

	// AppVersion 打这个包时的 Catflap 版本，只作展示/排查用
	AppVersion string `json:"appVersion"`

	CreatedAt time.Time `json:"createdAt"`

	// Modules 包里有哪些模块配置
	Modules []Module `json:"modules"`

	// Assets 包里有哪些上传资产（图标/壁纸）
	Assets []Asset `json:"assets"`

	/**
	 * Sensitive 这个包里含敏感信息。
	 *
	 * DDNS 的 Cloudflare token、auth 的 JWT 密钥、分组的口令哈希都在配置里 ——
	 * 备份包等于一份完整的凭据副本。标记出来是为了**界面上能明确警告**，
	 * 别让人随手把它传上网盘。
	 */
	Sensitive bool `json:"sensitive"`
}

type Module struct {
	// Name 模块名，对应 MODULES 里的 key（home / auth / proxy / stun / ddns …）
	Name string `json:"name"`
	// Label 界面上显示的中文名
	Label string `json:"label"`
	// File 包内的相对路径
	File string `json:"file"`
	// Size 原始字节数
	Size int64 `json:"size"`
	// Exists 打这个包时该模块的配置是否存在（不存在就不该让用户勾）
	Exists bool `json:"exists"`
}

type Asset struct {
	// Kind icon / wallpaper
	Kind string `json:"kind"`
	// Dir 原始目录（data/icon）
	Dir string `json:"dir"`
	// Count 文件数
	Count int `json:"count"`
	// Bytes 总字节数
	Bytes int64 `json:"bytes"`
}

// marshalManifest 序列化清单。导出写包和测试构造包共用一个实现 ——
// 两边各写一份的话，将来字段一改就会出现"造出来的包和真实包不一致"，
// 测试还全绿，这种假绿最坑。
func marshalManifest(m Manifest) ([]byte, error) {
	if m.Version <= 0 {
		m.Version = ManifestVersion
	}
	return json.MarshalIndent(m, "", "  ")
}
