package ddns_api

import (
	"github.com/chenggegedeid/catflap/middleware"
	"github.com/chenggegedeid/catflap/modules/ddns"
	"github.com/chenggegedeid/catflap/modules/ddns/model"
	"github.com/chenggegedeid/catflap/utils/res"

	"github.com/gin-gonic/gin"
)

type DdnsProviderAddRequest struct {
	Name       string                `json:"name"`
	Type       model.DNSProviderType `json:"type"`
	Credential map[string]string     `json:"credential"` // 凭证字段，key 因服务商而异
}

func (DdnsApi) DdnsProviderAddView(c *gin.Context) {
	cr := middleware.GetBindRequest[DdnsProviderAddRequest](c)

	p, err := ddns.Runtime.AddProvider(cr.Name, cr.Type, cr.Credential)
	if err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}

	res.OkWithData(ProviderView{
		ID:            p.ID,
		Name:          p.Name,
		Type:          p.Type,
		HasCredential: true,
	}, c)
}
