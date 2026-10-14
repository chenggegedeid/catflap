package dns

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/chenggegedeid/catflap/modules/ddns/model"
)

// Porkbun DNS
// 文档: https://porkbun.com/api/documentation/v3
const porkbunEndpoint = "https://porkbun.com/api/json/v3"

// Porkbun 客户端。凭据是 API Key + Secret Key，都放在请求体里。
type Porkbun struct {
	APIKey       string
	SecretAPIKey string
	httpClient   *http.Client
}

// NewPorkbun 创建客户端
func NewPorkbun(apiKey, secretAPIKey string) *Porkbun {
	return &Porkbun{
		APIKey:       apiKey,
		SecretAPIKey: secretAPIKey,
		httpClient: &http.Client{
			Timeout:   30 * time.Second,
			Transport: &http.Transport{Proxy: nil},
		},
	}
}

type porkbunRequest struct {
	APIKey       string `json:"apikey"`
	SecretAPIKey string `json:"secretapikey"`
	Name         string `json:"name,omitempty"`
	Type         string `json:"type,omitempty"`
	Content      string `json:"content"`
	TTL          string `json:"ttl"`
}

type porkbunResponse struct {
	Status string `json:"status"`
	// 失败时 Porkbun 把原因放在 message 里，成功时是空串
	Message string `json:"message,omitempty"`
}

// SetRecord 设置一条记录。
//
// Porkbun 的「按名字和类型改」接口在记录不存在时会失败，所以这里先改、
// 改不动再建 —— 两条分支都是正常路径，不是错误。
func (p *Porkbun) SetRecord(domain string, subDomain string, recordType model.DNSRecordType, ipAddr string, ttl int, proxied bool) error {
	// Porkbun 的 TTL 下限是 600，给 0 会被拒
	if ttl < 600 {
		ttl = 600
	}

	if err := p.edit(domain, subDomain, recordType, ipAddr, ttl); err != nil {
		// 只有「记录不存在」这种情况值得再试一次建；其它错误（凭据错、
		// 域名不在账号下）重试也只会再失败一次，不如直接把错误交上去。
		if err == errPorkbunNoRecord {
			return p.create(domain, subDomain, recordType, ipAddr, ttl)
		}
		return err
	}
	return nil
}

// errPorkbunNoRecord 标记「记录还不存在」，由 edit 返回、SetRecord 消费。
//
// 用一个哨兵错误而不是直接建：这样「凭据填错了」不会伪装成「记录不存在」
// 被吞掉 —— 后者会导致每次同步都白白多打一次创建接口。
var errPorkbunNoRecord = fmt.Errorf("porkbun: 记录不存在")

func (p *Porkbun) edit(domain, subDomain string, recordType model.DNSRecordType, ipAddr string, ttl int) error {
	var resp porkbunResponse
	err := p.post(fmt.Sprintf("/dns/editByNameType/%s/%s/%s", domain, string(recordType), subDomain),
		porkbunRequest{Content: ipAddr, TTL: fmt.Sprint(ttl)}, &resp)
	if err != nil {
		return err
	}
	if resp.Status != "SUCCESS" {
		if strings.Contains(strings.ToLower(resp.Message), "no record") {
			return errPorkbunNoRecord
		}
		return fmt.Errorf("修改记录失败: %s", resp.Message)
	}
	return nil
}

func (p *Porkbun) create(domain, subDomain string, recordType model.DNSRecordType, ipAddr string, ttl int) error {
	var resp porkbunResponse
	err := p.post(fmt.Sprintf("/dns/create/%s", domain), porkbunRequest{
		Name:    subDomain,
		Type:    string(recordType),
		Content: ipAddr,
		TTL:     fmt.Sprint(ttl),
	}, &resp)
	if err != nil {
		return err
	}
	if resp.Status != "SUCCESS" {
		return fmt.Errorf("新增记录失败: %s", resp.Message)
	}
	return nil
}

func (p *Porkbun) post(path string, body porkbunRequest, result *porkbunResponse) error {
	body.APIKey = p.APIKey
	body.SecretAPIKey = p.SecretAPIKey

	data, err := json.Marshal(body)
	if err != nil {
		return err
	}

	resp, err := p.httpClient.Post(porkbunEndpoint+path, "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("请求 Porkbun 失败: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Porkbun 返回 %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}
	if err := json.Unmarshal(raw, result); err != nil {
		return fmt.Errorf("解析 Porkbun 响应失败: %w", err)
	}
	return nil
}
