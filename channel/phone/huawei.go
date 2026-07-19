package phone

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
	notify.Register(notify.ChannelPhone, notify.ProviderHuawei, func(config json.RawMessage) (notify.Sender, error) {
		var c HuaweiConfig
		if err := json.Unmarshal(config, &c); err != nil {
			return nil, fmt.Errorf("%w: phone/huawei: %v", notify.ErrConfig, err)
		}
		if c.Endpoint == "" || c.AppKey == "" || c.AppSecret == "" {
			return nil, fmt.Errorf("%w: phone/huawei: endpoint/appKey/appSecret are required", notify.ErrConfig)
		}
		return &huaweiSender{config: c}, nil
	})
}

// HuaweiConfig 华为云语音（VoiceCall）配置
type HuaweiConfig struct {
	Endpoint   string `json:"endpoint"`   // API 地址
	AppKey     string `json:"appKey"`     // App Key
	AppSecret  string `json:"appSecret"`  // App Secret
	CallFrom   string `json:"callFrom"`   // 主叫显号
	TemplateId string `json:"templateId"` // TTS 模板 ID（可被 Extra.templateId 覆盖）
}

type huaweiSender struct {
	config HuaweiConfig
}

// Send 调用华为云语音 TTS 呼叫（WSSE 认证）
func (s *huaweiSender) Send(ctx context.Context, req *notify.SendRequest) (*notify.Result, error) {
	templateId := req.GetExtra("templateId", s.config.TemplateId)
	if templateId == "" {
		return nil, fmt.Errorf("%w: phone/huawei: templateId is required", notify.ErrConfig)
	}

	payload := map[string]interface{}{
		"call_type":          "VOICE_TTS",
		"caller":             s.config.CallFrom,
		"callee":             req.To,
		"display_name":       "告警通知",
		"status_callback":    "",
		"tts_template_id":    templateId,
		"tts_template_param": fmt.Sprintf("[%q]", req.Content),
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
		return nil, fmt.Errorf("%w: phone/huawei: %v", notify.ErrSend, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: phone/huawei: status %d, %s", notify.ErrSend, resp.StatusCode, truncate(string(body)))
	}
	return &notify.Result{Raw: truncate(string(body))}, nil
}
