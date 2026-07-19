package sms

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	notify "github.com/han3sui/iot-platform-notify"
	"github.com/han3sui/iot-platform-notify/internal/cloudsign"
)

const (
	tencentSMSHost    = "sms.tencentcloudapi.com"
	tencentSMSService = "sms"
	tencentSMSVersion = "2021-01-11"
	tencentSMSAction  = "SendSms"
)

func init() {
	notify.Register(notify.ChannelSMS, notify.ProviderTencent, func(config json.RawMessage) (notify.Sender, error) {
		var c TencentConfig
		if err := json.Unmarshal(config, &c); err != nil {
			return nil, fmt.Errorf("%w: sms/tencent: %v", notify.ErrConfig, err)
		}
		if c.SecretId == "" || c.SecretKey == "" || c.AppId == "" {
			return nil, fmt.Errorf("%w: sms/tencent: secretId/secretKey/appId are required", notify.ErrConfig)
		}
		if c.Region == "" {
			c.Region = "ap-guangzhou"
		}
		return &tencentSender{config: c}, nil
	})
}

// TencentConfig 腾讯云短信配置（TC3 v3 API，字段与基座渠道配置页对齐）
// 注：基座旧 adapter 使用已下线的 v2 API（appKey+sig），本实现按官方 SendSms 2021-01-11 重写
type TencentConfig struct {
	SecretId   string `json:"secretId"`   // Secret ID
	SecretKey  string `json:"secretKey"`  // Secret Key
	AppId      string `json:"appId"`      // 短信 SdkAppId
	Region     string `json:"region"`     // 区域，默认 ap-guangzhou
	SignName   string `json:"signName"`   // 短信签名（可被 Extra.signName 覆盖）
	TemplateId string `json:"templateId"` // 模板 ID（可被 Extra.templateId 覆盖）
}

type tencentSender struct {
	config TencentConfig
}

// tencentSMSResponse 腾讯云 SendSms 响应
type tencentSMSResponse struct {
	Response struct {
		SendStatusSet []struct {
			SerialNo    string `json:"SerialNo"`
			Code        string `json:"Code"`
			Message     string `json:"Message"`
			PhoneNumber string `json:"PhoneNumber"`
		} `json:"SendStatusSet"`
		Error *struct {
			Code    string `json:"Code"`
			Message string `json:"Message"`
		} `json:"Error"`
	} `json:"Response"`
}

// Send 调用腾讯云 SendSms（TC3-HMAC-SHA256），模板参数为 [Content]
func (s *tencentSender) Send(ctx context.Context, req *notify.SendRequest) (*notify.Result, error) {
	signName := req.GetExtra("signName", s.config.SignName)
	templateId := req.GetExtra("templateId", s.config.TemplateId)
	if signName == "" || templateId == "" {
		return nil, fmt.Errorf("%w: sms/tencent: signName and templateId are required", notify.ErrConfig)
	}

	body := map[string]interface{}{
		"PhoneNumberSet":   []string{normalizePhone(req.To)},
		"SmsSdkAppId":      s.config.AppId,
		"SignName":         signName,
		"TemplateId":       templateId,
		"TemplateParamSet": []string{req.Content},
	}
	jsonData, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	payload := string(jsonData)

	sig := cloudsign.TC3Sign(tencentSMSHost, tencentSMSService, payload, s.config.SecretId, s.config.SecretKey, time.Now())

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://"+tencentSMSHost, bytes.NewBufferString(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", sig.Authorization)
	httpReq.Header.Set("X-TC-Action", tencentSMSAction)
	httpReq.Header.Set("X-TC-Version", tencentSMSVersion)
	httpReq.Header.Set("X-TC-Timestamp", fmt.Sprintf("%d", sig.Timestamp))
	httpReq.Header.Set("X-TC-Region", s.config.Region)

	resp, err := notify.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: sms/tencent: %v", notify.ErrSend, err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var smsResp tencentSMSResponse
	if err := json.Unmarshal(respBody, &smsResp); err != nil {
		return nil, fmt.Errorf("%w: sms/tencent: decode response failed: %v", notify.ErrSend, err)
	}
	if smsResp.Response.Error != nil {
		return nil, fmt.Errorf("%w: sms/tencent: %s, %s", notify.ErrSend, smsResp.Response.Error.Code, smsResp.Response.Error.Message)
	}
	var serialNo string
	for _, st := range smsResp.Response.SendStatusSet {
		if st.Code != "Ok" {
			return nil, fmt.Errorf("%w: sms/tencent: %s, %s (%s)", notify.ErrSend, st.Code, st.Message, st.PhoneNumber)
		}
		serialNo = st.SerialNo
	}
	return &notify.Result{MessageID: serialNo, Raw: truncate(string(respBody))}, nil
}

// normalizePhone 号码规范化：无 + 前缀的国内号码补 +86
func normalizePhone(phone string) string {
	if strings.HasPrefix(phone, "+") {
		return phone
	}
	return "+86" + phone
}
