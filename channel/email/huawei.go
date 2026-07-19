package email

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	notify "github.com/han3sui/iot-platform-notify"
	"github.com/han3sui/iot-platform-notify/internal/cloudsign"
)

func init() {
	notify.Register(notify.ChannelEmail, notify.ProviderHuawei, func(config json.RawMessage) (notify.Sender, error) {
		var c HuaweiConfig
		if err := json.Unmarshal(config, &c); err != nil {
			return nil, fmt.Errorf("%w: email/huawei: %v", notify.ErrConfig, err)
		}
		if c.Endpoint == "" || c.AppKey == "" || c.AppSecret == "" {
			return nil, fmt.Errorf("%w: email/huawei: endpoint/appKey/appSecret are required", notify.ErrConfig)
		}
		return &huaweiSender{config: c}, nil
	})
}

// HuaweiConfig 华为云邮件（KooMessage/MSGSMS 邮件接口）配置
type HuaweiConfig struct {
	Endpoint  string `json:"endpoint"`  // API 地址
	AppKey    string `json:"appKey"`    // App Key
	AppSecret string `json:"appSecret"` // App Secret
	From      string `json:"fromEmail"` // 发送者邮箱
}

type huaweiSender struct {
	config HuaweiConfig
}

// Send 调用华为云邮件 API 发送 HTML 邮件（WSSE 认证）
func (s *huaweiSender) Send(ctx context.Context, req *notify.SendRequest) (*notify.Result, error) {
	payload := map[string]interface{}{
		"from":    s.config.From,
		"to":      []string{req.To},
		"subject": req.Subject,
		"content": map[string]string{
			"html": req.Content,
		},
	}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, s.config.Endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "WSSE realm=\"SDP\",profile=\"UsernameToken\",type=\"Appkey\"")
	httpReq.Header.Set("X-WSSE", cloudsign.HuaweiWSSEHeaderHMAC(s.config.AppKey, s.config.AppSecret))

	resp, err := notify.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: email/huawei: %v", notify.ErrSend, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: email/huawei: status %d, %s", notify.ErrSend, resp.StatusCode, truncate(string(body)))
	}
	return &notify.Result{Raw: string(body)}, nil
}
