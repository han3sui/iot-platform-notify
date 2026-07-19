package email

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	notify "github.com/han3sui/iot-platform-notify"
	"github.com/han3sui/iot-platform-notify/internal/cloudsign"
)

const (
	tencentSESHost    = "ses.tencentcloudapi.com"
	tencentSESService = "ses"
	tencentSESVersion = "2020-10-02"
	tencentSESAction  = "SendEmail"
)

func init() {
	notify.Register(notify.ChannelEmail, notify.ProviderTencent, func(config json.RawMessage) (notify.Sender, error) {
		var c TencentConfig
		if err := json.Unmarshal(config, &c); err != nil {
			return nil, fmt.Errorf("%w: email/tencent: %v", notify.ErrConfig, err)
		}
		if c.SecretId == "" || c.SecretKey == "" || c.From == "" {
			return nil, fmt.Errorf("%w: email/tencent: secretId/secretKey/fromEmail are required", notify.ErrConfig)
		}
		if c.Region == "" {
			c.Region = "ap-guangzhou"
		}
		return &tencentSender{config: c}, nil
	})
}

// TencentConfig 腾讯云邮件（SES）配置
type TencentConfig struct {
	SecretId  string `json:"secretId"`  // Secret ID
	SecretKey string `json:"secretKey"` // Secret Key
	From      string `json:"fromEmail"` // 发信地址（SES 控制台配置）
	Region    string `json:"region"`    // 区域，默认 ap-guangzhou
	TemplateID uint64 `json:"templateId,string,omitempty"` // 可选：SES 模板 ID（走模板发送）
}

type tencentSender struct {
	config TencentConfig
}

// tencentSESResponse 腾讯云 SES 响应
type tencentSESResponse struct {
	Response struct {
		MessageId string `json:"MessageId"`
		Error     *struct {
			Code    string `json:"Code"`
			Message string `json:"Message"`
		} `json:"Error"`
	} `json:"Response"`
}

// Send 调用腾讯云 SES SendEmail（TC3-HMAC-SHA256 签名，签名 payload 与 body 严格一致）
func (s *tencentSender) Send(ctx context.Context, req *notify.SendRequest) (*notify.Result, error) {
	// SES SendEmail 请求体：Simple 内容模式（HTML base64 不需要，Simple.Html 直接传原文即可）
	body := map[string]interface{}{
		"FromEmailAddress": s.config.From,
		"Destination":      []string{req.To},
		"Subject":          req.Subject,
		"Simple": map[string]string{
			"Html": req.Content,
		},
	}
	jsonData, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	payload := string(jsonData)

	sig := cloudsign.TC3Sign(tencentSESHost, tencentSESService, payload, s.config.SecretId, s.config.SecretKey, time.Now())

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://"+tencentSESHost, bytes.NewBufferString(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", sig.Authorization)
	httpReq.Header.Set("X-TC-Action", tencentSESAction)
	httpReq.Header.Set("X-TC-Version", tencentSESVersion)
	httpReq.Header.Set("X-TC-Timestamp", fmt.Sprintf("%d", sig.Timestamp))
	httpReq.Header.Set("X-TC-Region", s.config.Region)

	resp, err := notify.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: email/tencent: %v", notify.ErrSend, err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var sesResp tencentSESResponse
	if err := json.Unmarshal(respBody, &sesResp); err != nil {
		return nil, fmt.Errorf("%w: email/tencent: decode response failed: %v", notify.ErrSend, err)
	}
	if sesResp.Response.Error != nil {
		return nil, fmt.Errorf("%w: email/tencent: %s, %s", notify.ErrSend, sesResp.Response.Error.Code, sesResp.Response.Error.Message)
	}
	return &notify.Result{MessageID: sesResp.Response.MessageId, Raw: truncate(string(respBody))}, nil
}
