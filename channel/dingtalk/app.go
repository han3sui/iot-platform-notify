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

// dingtalkTokenInvalid 钉钉 access_token 失效类错误码：
// 40014 不合法的 token、42001 token 过期、40001 获取 token 的凭证非法
func dingtalkTokenInvalid(errcode int) bool {
	return errcode == 40014 || errcode == 42001 || errcode == 40001
}

// Send 发送钉钉工作通知文本消息；To 为钉钉 userid（多个用 , 分隔）。
// token 失效时清空缓存并重试一次（实例被 NewCached 复用，本地缓存可能滞后于钉钉侧失效）。
func (s *appSender) Send(ctx context.Context, req *notify.SendRequest) (*notify.Result, error) {
	result, err := s.sendOnce(ctx, req)
	if err != nil && result != nil && dingtalkTokenInvalid(result.errcode) {
		s.invalidateToken()
		result, err = s.sendOnce(ctx, req)
	}
	if err != nil {
		return nil, err
	}
	return &notify.Result{MessageID: strconv.FormatInt(result.taskId, 10)}, nil
}

// sendResult 单次发送的渠道方响应（errcode 用于 token 失效判定）
type sendResult struct {
	errcode int
	taskId  int64
}

func (s *appSender) sendOnce(ctx context.Context, req *notify.SendRequest) (*sendResult, error) {
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
		return &sendResult{errcode: result.Errcode},
			fmt.Errorf("%w: dingtalk/app: code %d, %s", notify.ErrSend, result.Errcode, result.Errmsg)
	}
	return &sendResult{taskId: result.TaskId}, nil
}

// invalidateToken 清空本地 token 缓存，下次发送强制重新获取
func (s *appSender) invalidateToken() {
	s.mu.Lock()
	s.accessToken = ""
	s.mu.Unlock()
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
