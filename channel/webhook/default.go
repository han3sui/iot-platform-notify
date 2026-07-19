// Package webhook 提供通用 HTTP Webhook 渠道 adapter
package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	notify "github.com/han3sui/iot-platform-notify"
)

func init() {
	notify.Register(notify.ChannelWebhook, notify.ProviderDefault, func(config json.RawMessage) (notify.Sender, error) {
		var c DefaultConfig
		if err := json.Unmarshal(config, &c); err != nil {
			return nil, fmt.Errorf("%w: webhook/default: %v", notify.ErrConfig, err)
		}
		if c.URL == "" {
			return nil, fmt.Errorf("%w: webhook/default: url is required", notify.ErrConfig)
		}
		return &defaultSender{config: c}, nil
	})
}

// DefaultConfig 通用 Webhook 配置
type DefaultConfig struct {
	URL     string            `json:"url"`     // 目标地址
	Headers map[string]string `json:"headers"` // 自定义请求头（可选）
}

type defaultSender struct {
	config DefaultConfig
}

// Send POST {"message": Content} 到目标地址；To 不使用
func (s *defaultSender) Send(ctx context.Context, req *notify.SendRequest) (*notify.Result, error) {
	payload := map[string]string{
		"message": req.Content,
	}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, s.config.URL, strings.NewReader(string(jsonData)))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	for key, value := range s.config.Headers {
		httpReq.Header.Set(key, value)
	}

	resp, err := notify.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: webhook/default: %v", notify.ErrSend, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: webhook/default: status %d", notify.ErrSend, resp.StatusCode)
	}
	return &notify.Result{}, nil
}
