package notify

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
)

// 渠道 key 常量
const (
	ChannelEmail    = "email"
	ChannelSMS      = "sms"
	ChannelPhone    = "phone"
	ChannelWeChat   = "wechat"
	ChannelDingTalk = "dingtalk"
	ChannelTelegram = "telegram"
	ChannelWebhook  = "webhook"
)

// 服务商 key 常量
const (
	ProviderSelfHosted = "selfhosted"
	ProviderAliyun     = "aliyun"
	ProviderHuawei     = "huawei"
	ProviderTencent    = "tencent"
	ProviderApp        = "app"
	ProviderRobot      = "robot"
	ProviderBot        = "bot"
	ProviderDefault    = "default"
)

// Factory 根据渠道配置构造 Sender
type Factory func(config json.RawMessage) (Sender, error)

var (
	registryMu sync.RWMutex
	registry   = map[string]Factory{}

	// senderCache 按 channel+provider+config 哈希缓存 Sender 实例，
	// 保证企微/钉钉应用类 adapter 的 access_token 缓存可复用
	senderCacheMu sync.Mutex
	senderCache   = map[string]Sender{}
)

func regKey(channel, provider string) string {
	return channel + "/" + provider
}

// Register 注册渠道工厂，各 adapter 包在 init() 中调用
func Register(channel, provider string, f Factory) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[regKey(channel, provider)] = f
}

// New 根据渠道/服务商和配置构造 Sender（每次新建实例，不走缓存）
func New(channel, provider string, config json.RawMessage) (Sender, error) {
	registryMu.RLock()
	f, ok := registry[regKey(channel, provider)]
	registryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %s/%s", ErrUnsupported, channel, provider)
	}
	return f(config)
}

// NewCached 与 New 相同，但按 channel+provider+config 哈希缓存实例，
// 推荐用于企微应用/钉钉应用等带 access_token 内部状态的渠道
func NewCached(channel, provider string, config json.RawMessage) (Sender, error) {
	h := sha256.Sum256(config)
	key := regKey(channel, provider) + "#" + hex.EncodeToString(h[:8])

	senderCacheMu.Lock()
	defer senderCacheMu.Unlock()
	if s, ok := senderCache[key]; ok {
		return s, nil
	}
	s, err := New(channel, provider, config)
	if err != nil {
		return nil, err
	}
	senderCache[key] = s
	return s, nil
}

// Channels 返回已注册的渠道 key 列表（channel/provider）
func Channels() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	keys := make([]string, 0, len(registry))
	for k := range registry {
		keys = append(keys, k)
	}
	return keys
}

// legacyMapping IoT 基座 DB 中文枚举 → 英文 channel/provider key
// 基座 notify_channel 表存的是中文值（如 "微信"+"企业微信应用消息"），
// 通过此映射切换到新包，无需 DB 迁移。
var legacyMapping = map[string][2]string{
	"微信|企业微信应用消息":    {ChannelWeChat, ProviderApp},
	"微信|企业微信群机器人":    {ChannelWeChat, ProviderRobot},
	"邮件|自建邮箱":         {ChannelEmail, ProviderSelfHosted},
	"邮件|阿里云邮箱":        {ChannelEmail, ProviderAliyun},
	"邮件|华为云邮箱":        {ChannelEmail, ProviderHuawei},
	"邮件|腾讯云邮箱":        {ChannelEmail, ProviderTencent},
	"短信|阿里云短信":        {ChannelSMS, ProviderAliyun},
	"短信|华为云短信":        {ChannelSMS, ProviderHuawei},
	"短信|腾讯云短信":        {ChannelSMS, ProviderTencent},
	"电话|阿里云电话":        {ChannelPhone, ProviderAliyun},
	"电话|华为云电话":        {ChannelPhone, ProviderHuawei},
	"电话|腾讯云电话":        {ChannelPhone, ProviderTencent},
	"Telegram|Telegram机器人": {ChannelTelegram, ProviderBot},
}

// LegacyMap 将基座的中文（发送类型, 服务商）映射为新包的 (channel, provider)
func LegacyMap(deliveryType, serviceProvider string) (channel, provider string, ok bool) {
	if v, exists := legacyMapping[deliveryType+"|"+serviceProvider]; exists {
		return v[0], v[1], true
	}
	return "", "", false
}
