// Package sms 提供短信渠道 adapter：aliyun / huawei / tencent
package sms

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	notify "github.com/han3sui/iot-platform-notify"
	"github.com/han3sui/iot-platform-notify/internal/cloudsign"
)

func init() {
	notify.Register(notify.ChannelSMS, notify.ProviderAliyun, func(config json.RawMessage) (notify.Sender, error) {
		var c AliyunConfig
		if err := json.Unmarshal(config, &c); err != nil {
			return nil, fmt.Errorf("%w: sms/aliyun: %v", notify.ErrConfig, err)
		}
		if c.AccessKeyId == "" || c.AccessKeySecret == "" {
			return nil, fmt.Errorf("%w: sms/aliyun: accessKeyId/accessKeySecret are required", notify.ErrConfig)
		}
		if c.Endpoint == "" {
			c.Endpoint = "https://dysmsapi.aliyuncs.com"
		}
		if c.Region == "" {
			c.Region = "cn-hangzhou"
		}
		return &aliyunSender{config: c}, nil
	})
}

// AliyunConfig 阿里云短信配置
type AliyunConfig struct {
	Endpoint        string `json:"endpoint"`        // 接入点，默认 https://dysmsapi.aliyuncs.com
	AccessKeyId     string `json:"accessKeyId"`     // AccessKey ID
	AccessKeySecret string `json:"accessKeySecret"` // AccessKey Secret
	Region          string `json:"region"`          // 区域，默认 cn-hangzhou
	SignName        string `json:"signName"`        // 短信签名（可被 Extra.signName 覆盖）
	TemplateCode    string `json:"templateCode"`    // 模板编码（可被 Extra.templateCode 覆盖）
}

type aliyunSender struct {
	config AliyunConfig
}

// Send 调用阿里云 SendSms；模板变量固定为 {"content": Content}，与基座行为一致
func (s *aliyunSender) Send(ctx context.Context, req *notify.SendRequest) (*notify.Result, error) {
	signName := req.GetExtra("signName", s.config.SignName)
	templateCode := req.GetExtra("templateCode", s.config.TemplateCode)
	if signName == "" || templateCode == "" {
		return nil, fmt.Errorf("%w: sms/aliyun: signName and templateCode are required", notify.ErrConfig)
	}

	tplParam, _ := json.Marshal(map[string]string{"content": req.Content})
	params := map[string]string{
		"AccessKeyId":      s.config.AccessKeyId,
		"Action":           "SendSms",
		"Format":           "JSON",
		"SignatureMethod":  "HMAC-SHA1",
		"SignatureNonce":   fmt.Sprintf("%d", time.Now().UnixNano()),
		"SignatureVersion": "1.0",
		"Timestamp":        time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		"Version":          "2017-05-25",
		"RegionId":         s.config.Region,
		"PhoneNumbers":     req.To,
		"SignName":         signName,
		"TemplateCode":     templateCode,
		"TemplateParam":    string(tplParam),
	}
	params["Signature"] = cloudsign.AliyunRPCSignature(params, s.config.AccessKeySecret)

	query := url.Values{}
	for k, v := range params {
		query.Set(k, v)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, s.config.Endpoint+"?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := notify.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: sms/aliyun: %v", notify.ErrSend, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: sms/aliyun: status %d, %s", notify.ErrSend, resp.StatusCode, truncate(string(body)))
	}
	// 阿里云 HTTP 200 但业务失败时返回 Code != "OK"
	var result struct {
		Code    string `json:"Code"`
		Message string `json:"Message"`
		BizId   string `json:"BizId"`
	}
	if err := json.Unmarshal(body, &result); err == nil && result.Code != "" && result.Code != "OK" {
		return nil, fmt.Errorf("%w: sms/aliyun: %s, %s", notify.ErrSend, result.Code, result.Message)
	}
	return &notify.Result{MessageID: result.BizId, Raw: truncate(string(body))}, nil
}

// truncate 截断过长响应
func truncate(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 500 {
		return s[:500]
	}
	return s
}
