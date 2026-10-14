package ddns_api

import (
	"github.com/chenggegedeid/catflap/middleware"
	"github.com/chenggegedeid/catflap/modules/ddns"
	"github.com/chenggegedeid/catflap/utils/res"

	"github.com/gin-gonic/gin"
)

type DdnsRecordDeleteRequest struct {
	ID uint `json:"id"`
}

func (DdnsApi) DdnsRecordDeleteView(c *gin.Context) {
	cr := middleware.GetBindRequest[DdnsRecordDeleteRequest](c)

	if err := ddns.Runtime.DeleteRecord(cr.ID); err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}

	res.OkWithMsg("记录已删除", c)
}
