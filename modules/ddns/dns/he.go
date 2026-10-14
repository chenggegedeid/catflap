package dns

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/chenggegedeid/catflap/modules/ddns/model"
)

// HE Hurricane Electric 的 dns.he.net。
//
// 它提供的是**动态 DNS 更新**接口（不是完整的记录管理接口），所以用法与
// 其它几家不同：那条 A 记录必须先在 dns.he.net 面板里建好，并为它单独设一个
// 更新密码；本程序只负责把 IP 写进去。
//
// 文档: https://dns.he.net/ (Dynamic DNS)
const heEndpoint = "https://dyn.dns.he.net/nic/update"

// HE 客户端
type HE struct {
	// Password 是在 dns.he.net 面板里给那条记录单独设的更新密码
	Password   string
	httpClient *http.Client
}

// NewHE 创建客户端
func NewHE(password string) *HE {
	return &HE{
		Password: password,
		httpClient: &http.Client{
			Timeout:   30 * time.Second,
			Transport: &http.Transport{Proxy: nil},
		},
	}
}

// SetRecord 把 IP 写进那条已存在的记录。
//
// 只支持 A 记录：HE 的动态更新接口只接受 IPv4（myip 参数不接受 IPv6，
// 也没有 AAAA 的对应流程）。所以要 AAAA 的话只能换一家服务商。
func (h *HE) SetRecord(domain string, subDomain string, recordType model.DNSRecordType, ipAddr string, ttl int, proxied bool) error {
	if recordType != model.DNSRecordTypeA {
		return fmt.Errorf("HE.net 只支持 A 记录（IPv4），不支持 %s", recordType)
	}

	host := subDomain
	if host == "" {
		host = domain
	} else {
		host = host + "." + domain
	}

	params := url.Values{}
	params.Set("hostname", host)
	params.Set("password", h.Password)
	params.Set("myip", ipAddr)

	resp, err := h.httpClient.Get(heEndpoint + "?" + params.Encode())
	if err != nil {
		return fmt.Errorf("请求 HE.net 失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	answer := strings.TrimSpace(string(body))

	// 成功时返回 "good <ip>" 或 "nochg <ip>"（IP 没变）；其余都是错误码开头
	if strings.HasPrefix(answer, "good") || strings.HasPrefix(answer, "nochg") {
		return nil
	}
	return fmt.Errorf("HE.net 更新失败: %s", truncate(answer, 200))
}
