// Package wechat 提供企业微信渠道 adapter：app（应用消息）/ robot（群机器人）
package wechat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	notify "github.com/han3sui/iot-platform-notify"
)

func init() {
	notify.Register(notify.ChannelWeChat, notify.ProviderApp, func(config json.RawMessage) (notify.Sender, error) {
		var c AppConfig
		if err := json.Unmarshal(config, &c); err != nil {
			return nil, fmt.Errorf("%w: wechat/app: %v", notify.ErrConfig, err)
		}
		if c.CorpID == "" || c.CorpSecret == "" || c.AgentID <= 0 {
			return nil, fmt.Errorf("%w: wechat/app: corpId/corpSecret/agentId are required", notify.ErrConfig)
		}
		return &appSender{config: c}, nil
	})
}

// AppConfig 企业微信应用消息配置
type AppConfig struct {
	CorpID     string `json:"corpId"`     // 企业 ID
	CorpSecret string `json:"corpSecret"` // 应用密钥
	AgentID    int64  `json:"agentId"`    // 应用 ID
}

// appSender 企业微信应用消息发送器（内部缓存 access_token，建议配合 NewCached 复用实例）
type appSender struct {
	config AppConfig

	mu           sync.Mutex
	accessToken  string
	tokenExpires time.Time
}

// Send 发送企业微信应用 markdown 消息；To 为企微 userid（多个用 | 分隔）
func (s *appSender) Send(ctx context.Context, req *notify.SendRequest) (*notify.Result, error) {
	token, err := s.getAccessToken(ctx)
	if err != nil {
		return nil, err
	}

	payload := map[string]interface{}{
		"touser":   req.To,
		"agentid":  s.config.AgentID,
		"msgtype":  "markdown",
		"markdown": map[string]string{"content": req.Content},
	}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	apiURL := fmt.Sprintf("https://qyapi.weixin.qq.com/cgi-bin/message/send?access_token=%s", token)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, strings.NewReader(string(jsonData)))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := notify.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: wechat/app: %v", notify.ErrSend, err)
	}
	defer resp.Body.Close()

	var result struct {
		Errcode int    `json:"errcode"`
		Errmsg  string `json:"errmsg"`
		Msgid   string `json:"msgid"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("%w: wechat/app: decode response failed: %v", notify.ErrSend, err)
	}
	if result.Errcode != 0 {
		return nil, fmt.Errorf("%w: wechat/app: code %d, %s", notify.ErrSend, result.Errcode, result.Errmsg)
	}
	return &notify.Result{MessageID: result.Msgid}, nil
}

// getAccessToken 获取企业微信访问令牌（提前 5 分钟过期，并发安全）
func (s *appSender) getAccessToken(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.accessToken != "" && time.Now().Before(s.tokenExpires) {
		return s.accessToken, nil
	}

	apiURL := fmt.Sprintf("https://qyapi.weixin.qq.com/cgi-bin/gettoken?corpid=%s&corpsecret=%s",
		s.config.CorpID, s.config.CorpSecret)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := notify.HTTPClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("%w: wechat/app: get token failed: %v", notify.ErrSend, err)
	}
	defer resp.Body.Close()

	var result struct {
		Errcode     int    `json:"errcode"`
		Errmsg      string `json:"errmsg"`
		AccessToken string `json:"access_token"`
		ExpiresIn   float64 `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("%w: wechat/app: decode token response failed: %v", notify.ErrSend, err)
	}
	if result.Errcode != 0 {
		return "", fmt.Errorf("%w: wechat/app: get token code %d, %s", notify.ErrSend, result.Errcode, result.Errmsg)
	}
	if result.AccessToken == "" {
		return "", fmt.Errorf("%w: wechat/app: access_token not found in response", notify.ErrSend)
	}

	s.accessToken = result.AccessToken
	// 比官方过期时间提前5分钟过期，确保安全
	s.tokenExpires = time.Now().Add(time.Duration(result.ExpiresIn-300) * time.Second)
	return s.accessToken, nil
}
