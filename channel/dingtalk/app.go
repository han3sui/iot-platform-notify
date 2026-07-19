package dingtalk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	notify "github.com/han3sui/iot-platform-notify"
)

func init() {
	notify.Register(notify.ChannelDingTalk, notify.ProviderApp, func(config json.RawMessage) (notify.Sender, error) {
		var c AppConfig
		if err := json.Unmarshal(config, &c); err != nil {
			return nil, fmt.Errorf("%w: dingtalk/app: %v", notify.ErrConfig, err)
		}
		if c.AppKey == "" || c.AppSecret == "" || c.AgentId <= 0 {
			return nil, fmt.Errorf("%w: dingtalk/app: appKey/appSecret/agentId are required", notify.ErrConfig)
		}
		return &appSender{config: c}, nil
	})
}

// AppConfig 钉钉工作通知配置
type AppConfig struct {
	AppKey    string `json:"appKey"`    // 应用 AppKey
	AppSecret string `json:"appSecret"` // 应用 AppSecret
	AgentId   int    `json:"agentId"`   // 应用 AgentID
}

// appSender 钉钉工作通知发送器（内部缓存 access_token，建议配合 NewCached 复用实例）
type appSender struct {
	config AppConfig

	mu           sync.Mutex
	accessToken  string
	tokenExpires time.Time
}

// Send 发送钉钉工作通知文本消息；To 为钉钉 userid（多个用 , 分隔）
func (s *appSender) Send(ctx context.Context, req *notify.SendRequest) (*notify.Result, error) {
	token, err := s.getAccessToken(ctx)
	if err != nil {
		return nil, err
	}

	msgContent := map[string]interface{}{
		"msgtype": "text",
		"text":    map[string]string{"content": req.Content},
	}
	msgBytes, err := json.Marshal(msgContent)
	if err != nil {
		return nil, err
	}

	form := url.Values{}
	form.Add("access_token", token)
	form.Add("agent_id", strconv.Itoa(s.config.AgentId))
	form.Add("userid_list", req.To)
	form.Add("msg", string(msgBytes))

	apiURL := "https://oapi.dingtalk.com/topapi/message/corpconversation/asyncsend_v2"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := notify.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: dingtalk/app: %v", notify.ErrSend, err)
	}
	defer resp.Body.Close()

	var result struct {
		Errcode int    `json:"errcode"`
		Errmsg  string `json:"errmsg"`
		TaskId  int64  `json:"task_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("%w: dingtalk/app: decode response failed: %v", notify.ErrSend, err)
	}
	if result.Errcode != 0 {
		return nil, fmt.Errorf("%w: dingtalk/app: code %d, %s", notify.ErrSend, result.Errcode, result.Errmsg)
	}
	return &notify.Result{MessageID: strconv.FormatInt(result.TaskId, 10)}, nil
}

// getAccessToken 获取钉钉访问令牌（提前 5 分钟过期，并发安全）
func (s *appSender) getAccessToken(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.accessToken != "" && time.Now().Before(s.tokenExpires) {
		return s.accessToken, nil
	}

	tokenURL := fmt.Sprintf("https://oapi.dingtalk.com/gettoken?appkey=%s&appsecret=%s",
		s.config.AppKey, s.config.AppSecret)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, tokenURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := notify.HTTPClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("%w: dingtalk/app: get token failed: %v", notify.ErrSend, err)
	}
	defer resp.Body.Close()

	var result struct {
		Errcode     int     `json:"errcode"`
		Errmsg      string  `json:"errmsg"`
		AccessToken string  `json:"access_token"`
		ExpiresIn   float64 `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("%w: dingtalk/app: decode token response failed: %v", notify.ErrSend, err)
	}
	if result.Errcode != 0 {
		return "", fmt.Errorf("%w: dingtalk/app: get token code %d, %s", notify.ErrSend, result.Errcode, result.Errmsg)
	}
	if result.AccessToken == "" {
		return "", fmt.Errorf("%w: dingtalk/app: access_token not found in response", notify.ErrSend)
	}

	s.accessToken = result.AccessToken
	// 比官方过期时间提前5分钟过期，确保安全
	s.tokenExpires = time.Now().Add(time.Duration(result.ExpiresIn-300) * time.Second)
	return s.accessToken, nil
}
