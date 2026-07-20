// Package phone 提供语音电话渠道 adapter：aliyun / huawei / tencent
package phone

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	notify "github.com/han3sui/iot-platform-notify"
	"github.com/han3sui/iot-platform-notify/internal/cloudsign"
	"github.com/han3sui/iot-platform-notify/internal/tplparams"
)

func init() {
	notify.Register(notify.ChannelPhone, notify.ProviderAliyun, func(config json.RawMessage) (notify.Sender, error) {
		var c AliyunConfig
		if err := json.Unmarshal(config, &c); err != nil {
			return nil, fmt.Errorf("%w: phone/aliyun: %v", notify.ErrConfig, err)
		}
		if c.AccessKeyId == "" || c.AccessKeySecret == "" {
			return nil, fmt.Errorf("%w: phone/aliyun: accessKeyId/accessKeySecret are required", notify.ErrConfig)
		}
		if c.Endpoint == "" {
			c.Endpoint = "https://dyvmsapi.aliyuncs.com"
		}
		if c.Region == "" {
			c.Region = "cn-hangzhou"
		}
		return &aliyunSender{config: c}, nil
	})
}

// AliyunConfig 阿里云语音（VMS）配置
type AliyunConfig struct {
	Endpoint         string `json:"endpoint"`         // 接入点，默认 https://dyvmsapi.aliyuncs.com
	AccessKeyId      string `json:"accessKeyId"`      // AccessKey ID
	AccessKeySecret  string `json:"accessKeySecret"`  // AccessKey Secret
	Region           string `json:"region"`           // 区域，默认 cn-hangzhou
	TtsCode          string `json:"ttsCode"`          // TTS 模板编码（可被 Extra.ttsCode 覆盖）
	CalledShowNumber string `json:"calledShowNumber"` // 被叫显号（可被 Extra.calledShowNumber 覆盖）
}

type aliyunSender struct {
	config AliyunConfig
}

// Send 调用阿里云 SingleCallByTts。
// TTS 变量优先取 Extra["templateParams"]（JSON 对象，命名参数），
// 未提供时回退为 {"message": Content}（与基座旧行为一致）。
func (s *aliyunSender) Send(ctx context.Context, req *notify.SendRequest) (*notify.Result, error) {
	ttsCode := req.GetExtra("ttsCode", s.config.TtsCode)
	if ttsCode == "" {
		return nil, fmt.Errorf("%w: phone/aliyun: ttsCode is required", notify.ErrConfig)
	}

	named, err := tplparams.ParseNamed(req.GetExtra("templateParams", ""))
	if err != nil {
		return nil, fmt.Errorf("%w: phone/aliyun: %v", notify.ErrConfig, err)
	}
	var ttsParam []byte
	if named != nil {
		ttsParam, _ = json.Marshal(named)
	} else {
		ttsParam, _ = json.Marshal(map[string]string{"message": req.Content})
	}
	params := map[string]string{
		"AccessKeyId":      s.config.AccessKeyId,
		"Action":           "SingleCallByTts",
		"Format":           "JSON",
		"RegionId":         s.config.Region,
		"SignatureMethod":  "HMAC-SHA1",
		"SignatureNonce":   fmt.Sprintf("%d", time.Now().UnixNano()),
		"SignatureVersion": "1.0",
		"Timestamp":        time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		"Version":          "2017-05-25",
		"CalledNumber":     req.To,
		"CalledShowNumber": req.GetExtra("calledShowNumber", s.config.CalledShowNumber),
		"TtsCode":          ttsCode,
		"TtsParam":         string(ttsParam),
	}
	params["Signature"] = cloudsign.AliyunRPCSignature(params, s.config.AccessKeySecret)

	query := url.Values{}
	for k, v := range params {
		query.Set(k, v)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, s.config.Endpoint+"?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := notify.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: phone/aliyun: %v", notify.ErrSend, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: phone/aliyun: status %d", notify.ErrSend, resp.StatusCode)
	}

	var result struct {
		Code    string `json:"Code"`
		Message string `json:"Message"`
		CallId  string `json:"CallId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("%w: phone/aliyun: decode response failed: %v", notify.ErrSend, err)
	}
	if result.Code != "" && result.Code != "OK" {
		return nil, fmt.Errorf("%w: phone/aliyun: %s, %s", notify.ErrSend, result.Code, result.Message)
	}
	return &notify.Result{MessageID: result.CallId}, nil
}

// truncate 截断过长响应
func truncate(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 500 {
		return s[:500]
	}
	return s
}
