package phone

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	notify "github.com/han3sui/iot-platform-notify"
	"github.com/han3sui/iot-platform-notify/internal/cloudsign"
	"github.com/han3sui/iot-platform-notify/internal/tplparams"
)

const (
	tencentVMSHost    = "vms.tencentcloudapi.com"
	tencentVMSService = "vms"
	tencentVMSVersion = "2020-09-02"
	tencentVMSAction  = "SendTtsVoice"
)

func init() {
	notify.Register(notify.ChannelPhone, notify.ProviderTencent, func(config json.RawMessage) (notify.Sender, error) {
		var c TencentConfig
		if err := json.Unmarshal(config, &c); err != nil {
			return nil, fmt.Errorf("%w: phone/tencent: %v", notify.ErrConfig, err)
		}
		if c.SecretId == "" || c.SecretKey == "" || c.AppId == "" {
			return nil, fmt.Errorf("%w: phone/tencent: secretId/secretKey/appId are required", notify.ErrConfig)
		}
		if c.Region == "" {
			c.Region = "ap-guangzhou"
		}
		if c.PlayTimes <= 0 {
			c.PlayTimes = 2
		}
		return &tencentSender{config: c}, nil
	})
}

// TencentConfig 腾讯云语音（VMS SendTtsVoice）配置
// 注：基座旧 adapter 的 TC3 签名实现有误（Action 误写为 DescribeInstances、
// 签名 payload 与实际 body 不一致、缺少 X-TC-* 头），本实现按官方规范重写
type TencentConfig struct {
	SecretId   string `json:"secretId"`   // Secret ID
	SecretKey  string `json:"secretKey"`  // Secret Key
	AppId      string `json:"appId"`      // 语音 SdkAppId
	Region     string `json:"region"`     // 区域，默认 ap-guangzhou
	TemplateId string `json:"templateId"` // TTS 模板 ID（可被 Extra.templateId 覆盖）
	PlayTimes  int    `json:"playTimes"`  // 播放次数，默认 2
}

type tencentSender struct {
	config TencentConfig
}

// tencentVMSResponse 腾讯云 SendTtsVoice 响应
type tencentVMSResponse struct {
	Response struct {
		SendStatus *struct {
			CallId    string `json:"CallId"`
			SessionId string `json:"SessionId"`
		} `json:"SendStatus"`
		Error *struct {
			Code    string `json:"Code"`
			Message string `json:"Message"`
		} `json:"Error"`
	} `json:"Response"`
}

// Send 调用腾讯云 VMS SendTtsVoice（TC3-HMAC-SHA256，签名 payload 与 body 严格一致）。
// TTS 参数优先取 Extra["templateParams"]（JSON 数组），未提供时回退为 [Content]。
func (s *tencentSender) Send(ctx context.Context, req *notify.SendRequest) (*notify.Result, error) {
	templateId := req.GetExtra("templateId", s.config.TemplateId)
	if templateId == "" {
		return nil, fmt.Errorf("%w: phone/tencent: templateId is required", notify.ErrConfig)
	}
	playTimes := s.config.PlayTimes
	if v := req.GetExtra("playTimes", ""); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			playTimes = n
		}
	}

	templateParams, err := tplparams.ParsePositional(req.GetExtra("templateParams", ""))
	if err != nil {
		return nil, fmt.Errorf("%w: phone/tencent: %v", notify.ErrConfig, err)
	}
	if templateParams == nil {
		templateParams = []string{req.Content}
	}

	body := map[string]interface{}{
		"TemplateId":       templateId,
		"TemplateParamSet": templateParams,
		"CalledNumber":     normalizePhone(req.To),
		"VoiceSdkAppid":    s.config.AppId,
		"PlayTimes":        playTimes,
	}
	jsonData, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	payload := string(jsonData)

	sig := cloudsign.TC3Sign(tencentVMSHost, tencentVMSService, payload, s.config.SecretId, s.config.SecretKey, time.Now())

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://"+tencentVMSHost, bytes.NewBufferString(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", sig.Authorization)
	httpReq.Header.Set("X-TC-Action", tencentVMSAction)
	httpReq.Header.Set("X-TC-Version", tencentVMSVersion)
	httpReq.Header.Set("X-TC-Timestamp", fmt.Sprintf("%d", sig.Timestamp))
	httpReq.Header.Set("X-TC-Region", s.config.Region)

	resp, err := notify.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: phone/tencent: %v", notify.ErrSend, err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var vmsResp tencentVMSResponse
	if err := json.Unmarshal(respBody, &vmsResp); err != nil {
		return nil, fmt.Errorf("%w: phone/tencent: decode response failed: %v", notify.ErrSend, err)
	}
	if vmsResp.Response.Error != nil {
		return nil, fmt.Errorf("%w: phone/tencent: %s, %s", notify.ErrSend, vmsResp.Response.Error.Code, vmsResp.Response.Error.Message)
	}
	var callId string
	if vmsResp.Response.SendStatus != nil {
		callId = vmsResp.Response.SendStatus.CallId
	}
	return &notify.Result{MessageID: callId, Raw: truncate(string(respBody))}, nil
}

// normalizePhone 号码规范化：无 + 前缀的国内号码补 +86
func normalizePhone(phone string) string {
	if strings.HasPrefix(phone, "+") {
		return phone
	}
	return "+86" + phone
}
