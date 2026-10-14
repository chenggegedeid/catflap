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

// DuckDNS 免费动态 DNS。
// 文档: https://www.duckdns.org/spec.jsp
const duckdnsEndpoint = "https://www.duckdns.org/update"

// DuckDNS 客户端。凭据只有一个 token。
type DuckDNS struct {
	Token      string
	httpClient *http.Client
}

// NewDuckDNS 创建客户端
func NewDuckDNS(token string) *DuckDNS {
	return &DuckDNS{
		Token: token,
		httpClient: &http.Client{
			Timeout:   30 * time.Second,
			Transport: &http.Transport{Proxy: nil},
		},
	}
}

// SetRecord 更新 IP。
//
// 两点与别家不同：
//   - 主机名是**不带 .duckdns.org 的那一段**（domains=mysub），
//     所以这里只用 subDomain，主域名那一栏填 duckdns.org 即可；
//   - 只支持 A 记录（IPv4）。DuckDNS 的 ipv6 要走另一个参数，本程序没接。
func (d *DuckDNS) SetRecord(domain string, subDomain string, recordType model.DNSRecordType, ipAddr string, ttl int, proxied bool) error {
	if recordType != model.DNSRecordTypeA {
		return fmt.Errorf("DuckDNS 只支持 A 记录（IPv4），不支持 %s", recordType)
	}
	if subDomain == "" {
		return fmt.Errorf("DuckDNS 需要填子域名（就是 xxxx.duckdns.org 里的 xxxx）")
	}

	params := url.Values{}
	params.Set("domains", subDomain)
	params.Set("token", d.Token)
	params.Set("ip", ipAddr)
	// verbose=true 时返回体里会带上当前 IP，便于判断到底写没写进去
	params.Set("verbose", "true")

	resp, err := d.httpClient.Get(duckdnsEndpoint + "?" + params.Encode())
	if err != nil {
		return fmt.Errorf("请求 DuckDNS 失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	// 成功返回以 OK 开头的几行；失败返回 "KO"
	answer := strings.TrimSpace(string(body))
	if strings.HasPrefix(answer, "OK") {
		return nil
	}
	return fmt.Errorf("DuckDNS 更新失败: %s", truncate(answer, 200))
}
