package backup_api

import (
	"fmt"

	"github.com/chenggegedeid/catflap/middleware"
	"github.com/chenggegedeid/catflap/modules/backup"
	"github.com/chenggegedeid/catflap/utils/res"

	"github.com/gin-gonic/gin"
)

type ExportRequest struct {
	Modules []string `json:"modules"`
	Assets  []string `json:"assets"`
}

/**
 * 导出备份：返回 zip 文件。
 *
 * 权限：必须登录。备份包里可能有 DDNS token、JWT 密钥、分组口令哈希 ——
 * 这是**整机凭据的副本**，绝不能让匿名访问拿到。
 */
func (BackupApi) ExportView(c *gin.Context) {
	cr := middleware.GetBindRequest[ExportRequest](c)

	out, err := backup.Export(backup.ExportOptions{Modules: cr.Modules, Assets: cr.Assets})
	if err != nil {
		res.FailWithMsg("导出失败:"+err.Error(), c)
		return
	}

	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, out.Filename))
	c.Header("Content-Type", "application/zip")
	c.Data(200, "application/zip", out.Data)
}

/**
 * 导入备份。
 *
 * 用 multipart 收文件（和上传壁纸/图标一致），不用 JSON ——
 * zip 是二进制，塞进 JSON 要 base64，白白大 33%。
 */
func (BackupApi) ImportView(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		res.FailWithMsg("缺少 file 字段", c)
		return
	}
	if file.Size > maxBackupSize {
		res.FailWithMsg(fmt.Sprintf("备份包太大（上限 %d MB）", maxBackupSize>>20), c)
		return
	}

	f, err := file.Open()
	if err != nil {
		res.FailWithMsg("读取上传文件失败", c)
		return
	}
	defer f.Close()

	raw := make([]byte, file.Size)
	if _, err := f.Read(raw); err != nil && err.Error() != "EOF" {
		res.FailWithMsg("读取上传文件失败", c)
		return
	}

	// 模块/资产的选择从 form 里取（和文件同一个 multipart）
	mods := c.PostFormArray("modules")
	assets := c.PostFormArray("assets")

	out, err := backup.Import(raw, backup.ImportOptions{Modules: mods, Assets: assets})
	if err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}

	res.OkWithData(out, c)
}

// 备份包含配置 + 图标 + 壁纸，给足上限
const maxBackupSize = 200 << 20

/**
 * 备份内容预览：上传一个包，先看清单再决定恢复哪些模块。
 *
 * 为什么要两步（先看再导）：导入是**覆盖性**操作，
 * 让人先看清包里有什么、再用勾选决定恢复范围，比"传上去直接全覆盖"稳妥。
 *
 * 注意 preview / import 走 multipart（文件 + form 字段），**不挂 BindJsonMiddleware** ——
 * 它们是 FormFile 而不是 JSON body，挂了反而会因为读不到 JSON 而报错。
 */
func (BackupApi) PreviewView(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		res.FailWithMsg("缺少 file 字段", c)
		return
	}
	if file.Size > maxBackupSize {
		res.FailWithMsg("备份包太大", c)
		return
	}

	f, err := file.Open()
	if err != nil {
		res.FailWithMsg("读取上传文件失败", c)
		return
	}
	defer f.Close()

	raw := make([]byte, file.Size)
	if _, err := f.Read(raw); err != nil && err.Error() != "EOF" {
		res.FailWithMsg("读取上传文件失败", c)
		return
	}

	mf, err := backup.Peek(raw)
	if err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithData(mf, c)
}
