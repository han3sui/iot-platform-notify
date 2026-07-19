// Package telegram 提供 Telegram Bot 渠道 adapter
package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	notify "github.com/han3sui/iot-platform-notify"
)

func init() {
	notify.Register(notify.ChannelTelegram, notify.ProviderBot, func(config json.RawMessage) (notify.Sender, error) {
		var c BotConfig
		if err := json.Unmarshal(config, &c); err != nil {
			return nil, fmt.Errorf("%w: telegram/bot: %v", notify.ErrConfig, err)
		}
		if c.BotToken == "" {
			return nil, fmt.Errorf("%w: telegram/bot: botToken is required", notify.ErrConfig)
		}
		return &botSender{config: c}, nil
	})
}

// BotConfig Telegram 机器人配置
type BotConfig struct {
	BotToken  string `json:"botToken"`  // Bot Token
	ParseMode string `json:"parseMode"` // 解析模式：Markdown/HTML（可选）
}

type botSender struct {
	config BotConfig
}

// Send 发送 Telegram 消息；To 为 chat_id
func (s *botSender) Send(ctx context.Context, req *notify.SendRequest) (*notify.Result, error) {
	payload := map[string]string{
		"chat_id": req.To,
		"text":    req.Content,
	}
	if s.config.ParseMode != "" {
		payload["parse_mode"] = s.config.ParseMode
	}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", s.config.BotToken)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, strings.NewReader(string(jsonData)))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := notify.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: telegram/bot: %v", notify.ErrSend, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: telegram/bot: status %d, %s", notify.ErrSend, resp.StatusCode, truncate(string(body)))
	}
	return &notify.Result{Raw: truncate(string(body))}, nil
}

// truncate 截断过长响应
func truncate(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 500 {
		return s[:500]
	}
	return s
}
