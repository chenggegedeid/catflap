package system_api

import (
	"github.com/chenggegedeid/catflap/middleware"
	"github.com/chenggegedeid/catflap/modules/update"
	"github.com/chenggegedeid/catflap/utils/res"

	"github.com/gin-gonic/gin"
)

// UpdateStatusView 当前版本、最新版本、能不能一键更新、下载进度
func (SystemApi) UpdateStatusView(c *gin.Context) {
	res.OkWithData(update.Runtime.Status(), c)
}

// UpdateCheckView 立刻去 GitHub 查一次
func (SystemApi) UpdateCheckView(c *gin.Context) {
	if err := update.Runtime.Check(c.Request.Context()); err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithData(update.Runtime.Status(), c)
}

// UpdateInstallView 开始下载新版本（内置加速源依次试），下载校验完会自己重启
func (SystemApi) UpdateInstallView(c *gin.Context) {
	if err := update.Runtime.Install(); err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithMsg("开始下载", c)
}

// UpdateUploadView 手动上传程序文件更新，下载不了 GitHub 或者要回退老版本时用
func (SystemApi) UpdateUploadView(c *gin.Context) {
	fh, err := c.FormFile("file")
	if err != nil {
		res.FailWithMsg("没收到文件", c)
		return
	}
	f, err := fh.Open()
	if err != nil {
		res.FailWithMsg("读取上传的文件失败", c)
		return
	}
	defer f.Close()
	if err := update.Runtime.InstallUpload(f); err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithMsg("已换上，正在重启", c)
}

type UpdateConfigRequest struct {
	Source string `json:"source"` // "" 内置 / "official" 官方 / "custom" 自定义
	Custom string `json:"custom"`

	// Proxy 下载更新时走的代理。要原样带回落盘 ——
	// SetConfig 是整份覆盖，这里少传就等于把代理清空，
	// 用户在界面上填了却保存不住，而且看不出为什么。
	Proxy string `json:"proxy"`
}

// UpdateConfigView 保存下载源与更新代理
func (SystemApi) UpdateConfigView(c *gin.Context) {
	cr := middleware.GetBindRequest[UpdateConfigRequest](c)
	if err := update.Runtime.SetConfig(update.Config{
		Source: cr.Source,
		Custom: cr.Custom,
		Proxy:  cr.Proxy,
	}); err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithData(update.Runtime.Status(), c)
}
