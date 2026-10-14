package ddns_api

import (
	"github.com/chenggegedeid/catflap/modules/ddns"
	"github.com/chenggegedeid/catflap/utils/res"

	"github.com/gin-gonic/gin"
)

// DdnsInterfaceListView 列出本机网卡，给「本地网卡」这个 IP 来源当下拉选项。
func (DdnsApi) DdnsInterfaceListView(c *gin.Context) {
	list, err := ddns.ListInterfaces()
	if err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithData(list, c)
}
