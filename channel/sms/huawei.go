package sms

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	notify "github.com/han3sui/iot-platform-notify"
	"github.com/han3sui/iot-platform-notify/internal/cloudsign"
	"github.com/han3sui/iot-platform-notify/internal/tplparams"
)

func init() {
	notify.Register(notify.ChannelSMS, notify.ProviderHuawei, func(config json.RawMessage) (notify.Sender, error) {
		var c HuaweiConfig
		if err := json.Unmarshal(config, &c); err != nil {
			return nil, fmt.Errorf("%w: sms/huawei: %v", notify.ErrConfig, err)
		}
		if c.Endpoint == "" || c.AccessKeyId == "" || c.AccessKeySecret == "" {
			return nil, fmt.Errorf("%w: sms/huawei: endpoint/accessKeyId/accessKeySecret are required", notify.ErrConfig)
		}
		return &huaweiSender{config: c}, nil
	})
}

// HuaweiConfig 华为云短信配置
type HuaweiConfig struct {
	Endpoint        string `json:"endpoint"`        // 接入点，如 https://smsapi.cn-north-4.myhuaweicloud.com:443
	AccessKeyId     string `json:"accessKeyId"`     // App Key
	AccessKeySecret string `json:"accessKeySecret"` // App Secret
	From            string `json:"from"`            // 签名通道号（可被 Extra.from 覆盖）
	TemplateId      string `json:"templateId"`      // 模板 ID（可被 Extra.templateId 覆盖）
}

type huaweiSender struct {
	config HuaweiConfig
}

// huaweiHTTPResult 华为云短信响应
type huaweiHTTPResult struct {
	Code        string `json:"code"`
	Description string `json:"description"`
	Result      []struct {
		SmsMsgId string `json:"smsMsgId"`
		Status   string `json:"status"`
	} `json:"result"`
}

// Send 调用华为云 batchSendSms。
// 模板参数优先取 Extra["templateParams"]（JSON 数组，位置参数），
// 未提供时回退为 [Content]（与基座旧行为一致）。
// 发送结果收敛：任一 Status != "000000" 即失败。
func (s *huaweiSender) Send(ctx context.Context, req *notify.SendRequest) (*notify.Result, error) {
	from := req.GetExtra("from", s.config.From)
	templateId := req.GetExtra("templateId", s.config.TemplateId)
	if from == "" || templateId == "" {
		return nil, fmt.Errorf("%w: sms/huawei: from and templateId are required", notify.ErrConfig)
	}

	templateParams, err := tplparams.ParsePositional(req.GetExtra("templateParams", ""))
	if err != nil {
		return nil, fmt.Errorf("%w: sms/huawei: %v", notify.ErrConfig, err)
	}
	if templateParams == nil {
		templateParams = []string{req.Content}
	}
	templateParasJSON, _ := json.Marshal(templateParams)

	values := url.Values{}
	values.Set("from", from)
	values.Set("to", req.To)
	values.Set("templateId", templateId)
	values.Set("templateParas", string(templateParasJSON))
	values.Set("statusCallback", "")

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		s.config.Endpoint+"/sms/batchSendSms/v1", strings.NewReader(values.Encode()))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpReq.Header.Set("Authorization", "WSSE realm=\"SDP\",profile=\"UsernameToken\",type=\"Appkey\"")
	httpReq.Header.Set("X-WSSE", cloudsign.HuaweiWSSEHeader(s.config.AccessKeyId, s.config.AccessKeySecret))

	resp, err := notify.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: sms/huawei: %v", notify.ErrSend, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: sms/huawei: status %d, %s", notify.ErrSend, resp.StatusCode, truncate(string(body)))
	}

	var result huaweiHTTPResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("%w: sms/huawei: decode response failed: %v", notify.ErrSend, err)
	}
	if result.Code != "000000" {
		return nil, fmt.Errorf("%w: sms/huawei: %s, %s", notify.ErrSend, result.Code, result.Description)
	}
	var msgID string
	for _, r := range result.Result {
		// 000000 为成功状态码
		if r.Status != "000000" {
			return nil, fmt.Errorf("%w: sms/huawei: send status %s", notify.ErrSend, r.Status)
		}
		msgID = r.SmsMsgId
	}
	return &notify.Result{MessageID: msgID, Raw: truncate(string(body))}, nil
}
