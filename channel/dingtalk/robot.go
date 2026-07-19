// Package dingtalk 提供钉钉渠道 adapter：robot（群机器人）/ app（工作通知）
package dingtalk

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	notify "github.com/han3sui/iot-platform-notify"
)

func init() {
	notify.Register(notify.ChannelDingTalk, notify.ProviderRobot, func(config json.RawMessage) (notify.Sender, error) {
		var c RobotConfig
		if err := json.Unmarshal(config, &c); err != nil {
			return nil, fmt.Errorf("%w: dingtalk/robot: %v", notify.ErrConfig, err)
		}
		if c.WebhookURL == "" {
			return nil, fmt.Errorf("%w: dingtalk/robot: webhookUrl is required", notify.ErrConfig)
		}
		return &robotSender{config: c}, nil
	})
}

// RobotConfig 钉钉群机器人配置
type RobotConfig struct {
	WebhookURL string `json:"webhookUrl"` // 群机器人 Webhook 地址
	Secret     string `json:"secret"`     // 加签密钥（可选，未配置则不加签）
}

type robotSender struct {
	config RobotConfig
}

// Send 发送钉钉群机器人文本消息；To 不使用（发到固定群）
func (s *robotSender) Send(ctx context.Context, req *notify.SendRequest) (*notify.Result, error) {
	requestURL := s.config.WebhookURL
	// 配置了加签密钥时按钉钉规范附加 timestamp+sign
	if s.config.Secret != "" {
		timestamp := fmt.Sprintf("%d", time.Now().UnixMilli())
		stringToSign := timestamp + "\n" + s.config.Secret

		mac := hmac.New(sha256.New, []byte(s.config.Secret))
		mac.Write([]byte(stringToSign))
		sign := url.QueryEscape(base64.StdEncoding.EncodeToString(mac.Sum(nil)))
		requestURL = fmt.Sprintf("%s&timestamp=%s&sign=%s", s.config.WebhookURL, timestamp, sign)
	}

	payload := map[string]interface{}{
		"msgtype": "text",
		"text":    map[string]string{"content": req.Content},
	}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, strings.NewReader(string(jsonData)))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := notify.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: dingtalk/robot: %v", notify.ErrSend, err)
	}
	defer resp.Body.Close()

	var result struct {
		Errcode int    `json:"errcode"`
		Errmsg  string `json:"errmsg"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("%w: dingtalk/robot: decode response failed: %v", notify.ErrSend, err)
	}
	if result.Errcode != 0 {
		return nil, fmt.Errorf("%w: dingtalk/robot: code %d, %s", notify.ErrSend, result.Errcode, result.Errmsg)
	}
	return &notify.Result{}, nil
}
