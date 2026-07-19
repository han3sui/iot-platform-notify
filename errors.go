package notify

import "errors"

var (
	// ErrUnsupported 渠道/服务商未注册
	ErrUnsupported = errors.New("notify: unsupported channel/provider")
	// ErrConfig 渠道配置非法（缺少必填字段或解析失败）
	ErrConfig = errors.New("notify: invalid config")
	// ErrSend 渠道方返回发送失败
	ErrSend = errors.New("notify: send failed")
)
