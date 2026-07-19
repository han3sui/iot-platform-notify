package wechat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	notify "github.com/han3sui/iot-platform-notify"
)

func init() {
	notify.Register(notify.ChannelWeChat, notify.ProviderRobot, func(config json.RawMessage) (notify.Sender, error) {
		var c RobotConfig
		if err := json.Unmarshal(config, &c); err != nil {
			return nil, fmt.Errorf("%w: wechat/robot: %v", notify.ErrConfig, err)
		}
		if c.WebhookURL == "" {
			return nil, fmt.Errorf("%w: wechat/robot: webhookUrl is required", notify.ErrConfig)
		}
		return &robotSender{config: c}, nil
	})
}

// RobotConfig 企业微信群机器人配置
type RobotConfig struct {
	WebhookURL string `json:"webhookUrl"` // 群机器人 Webhook 地址
}

type robotSender struct {
	config RobotConfig
}

// Send 发送企业微信群机器人文本消息；To 不使用（发到固定群）
func (s *robotSender) Send(ctx context.Context, req *notify.SendRequest) (*notify.Result, error) {
	payload := map[string]interface{}{
		"msgtype": "text",
		"text":    map[string]string{"content": req.Content},
	}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, s.config.WebhookURL, strings.NewReader(string(jsonData)))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := notify.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: wechat/robot: %v", notify.ErrSend, err)
	}
	defer resp.Body.Close()

	var result struct {
		Errcode int    `json:"errcode"`
		Errmsg  string `json:"errmsg"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("%w: wechat/robot: decode response failed: %v", notify.ErrSend, err)
	}
	if result.Errcode != 0 {
		return nil, fmt.Errorf("%w: wechat/robot: code %d, %s", notify.ErrSend, result.Errcode, result.Errmsg)
	}
	return &notify.Result{}, nil
}
