package system_api

import (
	"net/http"

	"github.com/chenggegedeid/catflap/core"

	"github.com/gin-gonic/gin"
)

// 版本号的注入点已经搬到 core.Version。
//
// 搬家的原因：更新模块需要知道自己是哪个版本，而它不能 import 这个包
// （本包 import 了它，会成环）。构建脚本相应地要改成：
//
//	go build -ldflags "-X github.com/chenggegedeid/catflap/core.Version=v1.0.0"
//
// 详见 core/version.go 里的说明。

type VersionResponse struct {
	Version string `json:"version"`
}

func (SystemApi) GetVersion(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": VersionResponse{
			Version: core.Version,
		},
	})
}
