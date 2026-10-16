package webhook

import (
	"errors"
	"fmt"
	httpUtils "github.com/chenggegedeid/catflap/utils/http_utils"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// queryValueRe 匹配 URL 查询串里的 `?k=v` / `&k=v`。
// 用来在错误消息里把 v 抹掉 —— 见 maskURLError。
// 值那一截排除引号：错误消息里 URL 是被包在双引号里的
// （`Post "http://...?token=xxx": dial tcp ...`），
// 不排除就会连后面的引号和正文一起吃掉，抹完的消息读不通。
var queryValueRe = regexp.MustCompile(`([?&])([^=&?\s]+)=[^&\s"]*`)

// maskURLError 把错误消息里 URL 上的查询参数值抹掉。
//
// 这是**安全约束**，不是文案美化：渠道凭证几乎都是拼在 URL 上的
// （PushPlus 是 ?token=xxx，Server 酱是 ?sendkey=xxx），
// 而 Go 的 http 包在失败时返回的是
//
//	Post "https://www.pushplus.plus/send?token=xxx": dial tcp ...
//
// URL 原样带在消息里。这个错误会被一路回给前端（通知页「测试发送」、
// 服务里「发一条测试通知」都是 res.FailWithMsg("发送失败："+err.Error())），
// 等于把渠道 token 显示到浏览器、写进网络面板和各类日志。
//
// 只抹值、留键名：用户还得知道"发给谁、因为什么失败"，
// 但拿不回凭证。用 errors.New 包一层，原始 err 的类型信息丢掉也没关系 ——
// 调用方只看消息。
func maskURLError(err error) error {
	if err == nil {
		return nil
	}
	return errors.New(queryValueRe.ReplaceAllString(err.Error(), "${1}${2}=***"))
}

// Send 按配置来发送一次 webhook
func Send(cfg WebhookConfig, fields map[string]string) (respBody string, err error) {
	replacer := buildReplacer(fields) //构造成 #{key} -> value 的替换器

	// 整理参数
	method := strings.ToUpper(strings.TrimSpace(cfg.Method))
	if method == "" {
		method = http.MethodGet
	}
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead, http.MethodOptions:
	default:
		return "", fmt.Errorf("不支持的请求方法: %s", method)
	}

	rawURL := strings.TrimSpace(replacer.Replace(cfg.URL))
	if rawURL == "" {
		return "", fmt.Errorf("webhook URL 为空")
	}
	body := replacer.Replace(cfg.Body)

	// 构建客户端
	//
	// 这里原来没判错误：buildClient 失败时返回的是 nil client，
	// 下面 sendOnce 里 client.Do(req) 就是空指针 panic（代理配错必崩）。
	// 顺手把错误也抹一遍 —— 代理地址常常写成 http://user:pass@host，
	// url.Parse 的报错会把整条地址（含密码）原样带出来。
	client, err := buildClient(cfg.Proxy)
	if err != nil {
		return "", maskURLError(err)
	}

	for i := 0; i < 3; i++ {
		respBody, err = sendOnce(client, method, rawURL, body, cfg, replacer)
		if err == nil {
			return respBody, nil
		}

		delay := time.Duration(1<<i) * time.Second
		time.Sleep(delay)
	}
	return respBody, err
}

// sendOnce 发送一次 webhook 请求并判定是否成功
func sendOnce(client *http.Client, method, rawURL, body string, cfg WebhookConfig, replacer *strings.Replacer) (string, error) {
	var reqBody io.Reader
	if method != http.MethodGet && body != "" {
		reqBody = strings.NewReader(body)
	}

	req, err := http.NewRequest(method, rawURL, reqBody)
	if err != nil {
		return "", maskURLError(err)
	}
	applyHeaders(req, cfg.Headers, replacer) // 写入header

	resp, err := client.Do(req)
	if err != nil {
		return "", maskURLError(err)
	}

	data, httpErr := httpUtils.GetHTTPResponseBody(resp)
	respBody := string(data)
	if httpErr != nil {
		// 非 2xx,GetHTTPResponseBody 已带上状态码与返回内容
		return respBody, httpErr
	}

	// 字符串检测
	if !cfg.DisableSuccessCheck {
		needle := strings.TrimSpace(cfg.SuccessContains)
		if needle != "" && !strings.Contains(respBody, needle) {
			return respBody, fmt.Errorf("返回体未包含成功字符串: %s", needle)
		}
	}
	return respBody, nil
}

// buildReplacer 把 fields 构造成 #{key} -> value 的替换器
func buildReplacer(fields map[string]string) *strings.Replacer {
	pairs := make([]string, 0, len(fields)*2)

	for k, v := range fields {
		pairs = append(pairs, "#{"+k+"}", v)
	}

	return strings.NewReplacer(pairs...)
}

// buildClient 构建 webhook 请求客户端
func buildClient(proxy string) (*http.Client, error) {
	transport := &http.Transport{Proxy: nil}
	// 解析http代理
	if p := strings.TrimSpace(proxy); p != "" {
		u, err := url.Parse(p)
		if err != nil {
			return nil, fmt.Errorf("代理地址无效: %w", err)
		}
		transport.Proxy = http.ProxyURL(u)
	}

	return &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
	}, nil
}

// applyHeaders 逐行解析 "Key: Value"，value 同样做占位符替换
func applyHeaders(req *http.Request, headers string, replacer *strings.Replacer) {
	for _, line := range strings.Split(headers, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		idx := strings.Index(line, ":")
		if idx <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(replacer.Replace(line[idx+1:]))
		if key != "" {
			req.Header.Set(key, val)
		}
	}
}
