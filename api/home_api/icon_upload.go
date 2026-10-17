package home_api

import (
	"fmt"
	"github.com/chenggegedeid/catflap/utils/res"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const iconDir = "data/icon"

var allowedIconExt = map[string]struct{}{
	".svg": {}, ".png": {}, ".ico": {}, ".jpg": {}, ".jpeg": {}, ".webp": {},
}

// 5 MiB
const maxIconSize = 5 << 20

// 上传图标,返回相对路径如 data/icon/xxx.svg
func (HomeApi) IconUploadView(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		res.FailWithMsg("缺少 file 字段", c)
		return
	}
	if file.Size > maxIconSize {
		res.FailWithMsg("文件大小超过 5MB", c)
		return
	}
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if _, ok := allowedIconExt[ext]; !ok {
		res.FailWithMsg("仅支持 svg/png/ico/jpg/webp", c)
		return
	}

	name := fmt.Sprintf("%d%s", time.Now().UnixNano(), ext)
	relPath := filepath.ToSlash(filepath.Join(iconDir, name))

	/**
	 * 目录必须自己建。
	 *
	 * 壁纸上传那边有 MkdirAll，图标这边**漏了** —— 于是全新部署时
	 * `data/icon` 还不存在，第一次上传必定失败（SaveUploadedFile 不会
	 * 自动创建父目录）。老环境因为目录已经存在所以看不出来，
	 * 只有在全新环境才会暴露。
	 */
	if err := os.MkdirAll(iconDir, 0755); err != nil {
		res.FailWithMsg("创建目录失败:"+err.Error(), c)
		return
	}

	if err := c.SaveUploadedFile(file, relPath); err != nil {
		res.FailWithMsg("保存失败:"+err.Error(), c)
		return
	}
	res.OkWithData(gin.H{"path": relPath}, c)
}
