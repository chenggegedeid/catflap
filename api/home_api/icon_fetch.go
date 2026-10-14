package home_api

import (
	"github.com/chenggegedeid/catflap/middleware"
	"github.com/chenggegedeid/catflap/modules/home"
	"github.com/chenggegedeid/catflap/utils/res"

	"github.com/gin-gonic/gin"
)

type IconFetchRequest struct {
	URL string `json:"url"`
}

// 从给定 URL 抓 favicon,返回保存到 data/icon/ 的相对路径。
func (HomeApi) IconFetchView(c *gin.Context) {
	cr := middleware.GetBindRequest[IconFetchRequest](c)
	path, err := home.FetchIconFromURL(cr.URL)
	if err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithData(gin.H{"path": path}, c)
}
