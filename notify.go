// Package notify 提供统一的多渠道通知发送能力。
//
// 设计原则：
//   - 包只负责「发送」，渠道密钥/接收人/模板管理由调用方（业务系统）负责
//   - 各渠道 adapter 通过 init() 自注册到 registry，消费方按需 blank import
//   - 配置以 json.RawMessage 传入，与调用方的存储方式（DB/配置文件）解耦
package notify

import (
	"context"
	"net/http"
	"time"
)

// Sender 统一发送接口，由各渠道 adapter 实现
type Sender interface {
	// Send 发送一条消息给单个接收地址
	Send(ctx context.Context, req *SendRequest) (*Result, error)
}

// SendRequest 单次发送请求
type SendRequest struct {
	To      string            `json:"to"`      // 接收地址：邮箱/手机号/企微userid/chatID等，部分渠道（群机器人/webhook）可为空
	Subject string            `json:"subject"` // 标题（邮件类渠道使用）
	Content string            `json:"content"` // 已渲染的消息正文
	Extra   map[string]string `json:"extra"`   // 渠道特有参数（如短信 templateId、signName、语音 playTimes）
}

// GetExtra 读取 Extra 参数，不存在时返回默认值
func (r *SendRequest) GetExtra(key, def string) string {
	if r.Extra != nil {
		if v, ok := r.Extra[key]; ok && v != "" {
			return v
		}
	}
	return def
}

// Result 发送结果
type Result struct {
	MessageID string `json:"message_id"` // 渠道方返回的消息ID（如有）
	Raw       string `json:"raw"`        // 渠道方原始响应（排障用）
}

// HTTPClient 包级共享 HTTP 客户端，调用方可在启动时替换（如自定义超时/代理）
var HTTPClient = &http.Client{Timeout: 10 * time.Second}
