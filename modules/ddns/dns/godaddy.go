package dns

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/chenggegedeid/catflap/modules/ddns/model"
)

// GoDaddy DNS
// 文档: https://developer.godaddy.com/doc/endpoint/domains#/v1/recordReplace
const godaddyEndpoint = "https://api.godaddy.com/v1/domains"

// GoDaddy 客户端。凭据是 API Key + Secret，配对成 `sso-key key:secret` 使用。
type GoDaddy struct {
	APIKey     string
	APISecret  string
	httpClient *http.Client
}

// NewGoDaddy 创建客户端
func NewGoDaddy(apiKey, apiSecret string) *GoDaddy {
	return &GoDaddy{
		APIKey:    apiKey,
		APISecret: apiSecret,
		httpClient: &http.Client{
			Timeout:   30 * time.Second,
			Transport: &http.Transport{Proxy: nil},
		},
	}
}

// godaddyRecord PUT 过去的那一条。接口收的是**数组**，即使只改一条。
type godaddyRecord struct {
	Data string `json:"data"`
	TTL  int    `json:"ttl"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// SetRecord 设置一条记录。
//
// GoDaddy 的 replace 语义正好合用：按「域名 + 类型 + 主机名」定位，存在就覆盖、
// 不存在就新建，一个 PUT 搞定，不需要先查列表再判断。
func (g *GoDaddy) SetRecord(domain string, subDomain string, recordType model.DNSRecordType, ipAddr string, ttl int, proxied bool) error {
	name := subDomain
	if name == "" {
		name = "@"
	}
	if ttl <= 0 {
		// GoDaddy 不接受 0，最小 600
		ttl = 600
	}

	body, err := json.Marshal([]godaddyRecord{{
		Data: ipAddr,
		TTL:  ttl,
		Name: name,
		Type: string(recordType),
	}})
	if err != nil {
		return err
	}

	apiURL := fmt.Sprintf("%s/%s/records/%s/%s",
		godaddyEndpoint, domain, string(recordType), name)

	req, err := http.NewRequest(http.MethodPut, apiURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "sso-key "+g.APIKey+":"+g.APISecret)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("请求 GoDaddy 失败: %w", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("GoDaddy 返回 %d: %s", resp.StatusCode, truncate(string(data), 200))
	}
	return nil
}

// truncate 错误信息里只留一段，避免把整个响应（可能很长）塞进日志
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
