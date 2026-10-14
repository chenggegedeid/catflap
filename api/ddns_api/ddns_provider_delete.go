package ddns_api

import (
	"github.com/chenggegedeid/catflap/middleware"
	"github.com/chenggegedeid/catflap/modules/ddns"
	"github.com/chenggegedeid/catflap/utils/res"

	"github.com/gin-gonic/gin"
)

type DdnsProviderDeleteRequest struct {
	ID uint `json:"id"`
}

func (DdnsApi) DdnsProviderDeleteView(c *gin.Context) {
	cr := middleware.GetBindRequest[DdnsProviderDeleteRequest](c)

	if err := ddns.Runtime.DeleteProvider(cr.ID); err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}

	res.OkWithMsg("服务商已删除", c)
}
