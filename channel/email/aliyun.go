package email

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	notify "github.com/han3sui/iot-platform-notify"
	"github.com/han3sui/iot-platform-notify/internal/cloudsign"
)

func init() {
	notify.Register(notify.ChannelEmail, notify.ProviderAliyun, func(config json.RawMessage) (notify.Sender, error) {
		var c AliyunConfig
		if err := json.Unmarshal(config, &c); err != nil {
			return nil, fmt.Errorf("%w: email/aliyun: %v", notify.ErrConfig, err)
		}
		if c.AccessKeyId == "" || c.AccessKeySecret == "" || c.AccountName == "" {
			return nil, fmt.Errorf("%w: email/aliyun: accessKeyId/accessKeySecret/accountName are required", notify.ErrConfig)
		}
		if c.Endpoint == "" {
			c.Endpoint = "https://dm.aliyuncs.com"
		}
		if c.Region == "" {
			c.Region = "cn-hangzhou"
		}
		return &aliyunSender{config: c}, nil
	})
}

// AliyunConfig 阿里云邮件推送（DirectMail）配置
type AliyunConfig struct {
	Endpoint        string `json:"endpoint"`        // API 接入点，默认 https://dm.aliyuncs.com
	AccessKeyId     string `json:"accessKeyId"`     // AccessKey ID
	AccessKeySecret string `json:"accessKeySecret"` // AccessKey Secret
	AccountName     string `json:"accountName"`     // 发信地址（管理控制台配置的发信地址）
	FromAlias       string `json:"fromName"`        // 发送者名称（可选）
	Region          string `json:"region"`          // 区域，默认 cn-hangzhou
}

type aliyunSender struct {
	config AliyunConfig
}

// Send 调用阿里云 DirectMail SingleSendMail 发送 HTML 邮件
func (s *aliyunSender) Send(ctx context.Context, req *notify.SendRequest) (*notify.Result, error) {
	params := map[string]string{
		"AccessKeyId":      s.config.AccessKeyId,
		"Action":           "SingleSendMail",
		"Format":           "JSON",
		"SignatureMethod":  "HMAC-SHA1",
		"SignatureNonce":   fmt.Sprintf("%d", time.Now().UnixNano()),
		"SignatureVersion": "1.0",
		"Timestamp":        time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		"Version":          "2015-11-23",
		"RegionId":         s.config.Region,
		"AccountName":      s.config.AccountName,
		"AddressType":      "1",
		"FromAlias":        s.config.FromAlias,
		"Subject":          req.Subject,
		"ToAddress":        req.To,
		"HtmlBody":         req.Content,
		"ReplyToAddress":   "false",
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
		return nil, fmt.Errorf("%w: email/aliyun: %v", notify.ErrSend, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: email/aliyun: status %d, %s", notify.ErrSend, resp.StatusCode, truncate(string(body)))
	}
	return &notify.Result{Raw: string(body)}, nil
}

// truncate 截断过长的响应，避免日志膨胀
func truncate(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 500 {
		return s[:500]
	}
	return s
}
