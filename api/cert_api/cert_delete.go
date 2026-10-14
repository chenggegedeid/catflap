package cert_api

import (
	"github.com/chenggegedeid/catflap/middleware"
	"github.com/chenggegedeid/catflap/modules/cert"
	"github.com/chenggegedeid/catflap/utils/res"

	"github.com/gin-gonic/gin"
)

type CertDeleteRequest struct {
	ID uint `json:"id" binding:"required"`
}

// CertDeleteView 删除证书，连同 data/cert/{id}/ 下的 PEM 和 ACME 账户私钥
func (CertApi) CertDeleteView(c *gin.Context) {
	cr := middleware.GetBindRequest[CertDeleteRequest](c)

	if err := cert.RemoveCertificate(cr.ID); err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithMsg("删除成功", c)
}
