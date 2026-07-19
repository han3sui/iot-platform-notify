// Package email 提供邮件渠道 adapter：selfhosted（SMTP）/ aliyun / huawei / tencent
package email

import (
	"context"
	"encoding/json"
	"fmt"

	notify "github.com/han3sui/iot-platform-notify"
	"gopkg.in/gomail.v2"
)

func init() {
	notify.Register(notify.ChannelEmail, notify.ProviderSelfHosted, func(config json.RawMessage) (notify.Sender, error) {
		var c SelfHostedConfig
		if err := json.Unmarshal(config, &c); err != nil {
			return nil, fmt.Errorf("%w: email/selfhosted: %v", notify.ErrConfig, err)
		}
		if c.Host == "" || c.From == "" {
			return nil, fmt.Errorf("%w: email/selfhosted: host and from are required", notify.ErrConfig)
		}
		return &selfHostedSender{config: c}, nil
	})
}

// SelfHostedConfig 自建 SMTP 邮箱配置（字段与基座渠道配置页对齐）
type SelfHostedConfig struct {
	Host      string `json:"host"`
	Port      int64  `json:"port"`
	Username  string `json:"username"`
	Password  string `json:"password"`
	From      string `json:"fromEmail"` // 发送者邮箱（基座字段名 fromEmail）
	FromName  string `json:"fromName"`  // 发送者名称（可选）
	EnableTLS bool   `json:"ssl"`       // 启用SSL（基座字段名 ssl）
}

type selfHostedSender struct {
	config SelfHostedConfig
}

// Send 通过 SMTP 发送 HTML 邮件
func (s *selfHostedSender) Send(ctx context.Context, req *notify.SendRequest) (*notify.Result, error) {
	m := gomail.NewMessage()
	if s.config.FromName != "" {
		m.SetHeader("From", m.FormatAddress(s.config.From, s.config.FromName))
	} else {
		m.SetHeader("From", s.config.From)
	}
	m.SetHeader("To", req.To)
	m.SetHeader("Subject", req.Subject)
	m.SetBody("text/html", req.Content)

	d := gomail.NewDialer(s.config.Host, int(s.config.Port), s.config.Username, s.config.Password)
	if !s.config.EnableTLS {
		d.SSL = false
		d.TLSConfig = nil
	}

	// gomail 不支持 ctx，遵循基座原有行为直接发送
	if err := d.DialAndSend(m); err != nil {
		return nil, fmt.Errorf("%w: email/selfhosted: %v", notify.ErrSend, err)
	}
	return &notify.Result{}, nil
}
