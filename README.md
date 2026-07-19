# iot-platform-notify

IoT 平台系多渠道通知发送包，从 `iot-platform` 基座 `internal/notify` 抽离，供基座与各上层业务系统（微电网等）共用。

- **只负责发送**：渠道密钥、接收人、通知组、模板管理由调用方（业务系统）自行存储和维护
- **统一接口**：所有渠道实现 `Sender.Send(ctx, req)`，配置以 JSON 传入
- **按需引入**：渠道 adapter 通过 blank import 注册；`template`（goja）、`queue`（go-redis）为独立子包，不用不引

## 安装

私有仓库需配置：

```bash
go env -w GOPRIVATE=github.com/han3sui/*
# HTTPS 拉取需配置凭证，或改用 SSH：
git config --global url."git@github.com:".insteadOf "https://github.com/"

go get github.com/han3sui/iot-platform-notify@v0.1.0
```

## 快速开始

```go
import (
    notify "github.com/han3sui/iot-platform-notify"
    _ "github.com/han3sui/iot-platform-notify/channel/all" // 注册全部渠道，或按需单独 import
)

sender, err := notify.New(notify.ChannelWeChat, notify.ProviderRobot,
    json.RawMessage(`{"webhookUrl":"https://qyapi.weixin.qq.com/..."}`))
result, err := sender.Send(ctx, &notify.SendRequest{
    To:      "",          // 群机器人类渠道不需要
    Subject: "",          // 邮件类渠道需要
    Content: "告警内容",
})
```

带 access_token 内部状态的渠道（企微应用、钉钉应用）建议用 `notify.NewCached` 复用实例，避免每次发送重新获取 token。

完整示例见 [examples/basic](examples/basic/main.go)。

## 渠道清单与配置字段

| channel | provider | 说明 | config 字段 | To 含义 |
|---------|----------|------|-------------|---------|
| `email` | `selfhosted` | 自建 SMTP | `host` `port` `username` `password` `fromEmail` `fromName` `ssl` | 收件邮箱 |
| `email` | `aliyun` | 阿里云邮件推送 | `accessKeyId` `accessKeySecret` `accountName` `fromName` `region` `endpoint` | 收件邮箱 |
| `email` | `huawei` | 华为云邮件 | `endpoint` `appKey` `appSecret` `fromEmail` | 收件邮箱 |
| `email` | `tencent` | 腾讯云 SES | `secretId` `secretKey` `fromEmail` `region` | 收件邮箱 |
| `sms` | `aliyun` | 阿里云短信 | `accessKeyId` `accessKeySecret` `region` `signName` `templateCode` `endpoint` | 手机号 |
| `sms` | `huawei` | 华为云短信 | `endpoint` `accessKeyId` `accessKeySecret` `from` `templateId` | 手机号 |
| `sms` | `tencent` | 腾讯云短信（v3 API） | `secretId` `secretKey` `appId` `region` `signName` `templateId` | 手机号（自动补 +86） |
| `phone` | `aliyun` | 阿里云语音 TTS | `accessKeyId` `accessKeySecret` `region` `ttsCode` `calledShowNumber` `endpoint` | 手机号 |
| `phone` | `huawei` | 华为云语音 TTS | `endpoint` `appKey` `appSecret` `callFrom` `templateId` | 手机号 |
| `phone` | `tencent` | 腾讯云语音 VMS | `secretId` `secretKey` `appId` `region` `templateId` `playTimes` | 手机号（自动补 +86） |
| `wechat` | `app` | 企业微信应用消息 | `corpId` `corpSecret` `agentId` | 企微 userid（`\|` 分隔多人） |
| `wechat` | `robot` | 企业微信群机器人 | `webhookUrl` | 不使用 |
| `dingtalk` | `app` | 钉钉工作通知 | `appKey` `appSecret` `agentId` | 钉钉 userid（`,` 分隔多人） |
| `dingtalk` | `robot` | 钉钉群机器人 | `webhookUrl` `secret`（加签可选） | 不使用 |
| `telegram` | `bot` | Telegram Bot | `botToken` `parseMode`（可选） | chat_id |
| `webhook` | `default` | 通用 HTTP POST | `url` `headers`（可选） | 不使用 |

短信/语音的模板参数（`signName`/`templateId`/`templateCode`/`ttsCode` 等）可放在渠道 config 里，也可通过 `SendRequest.Extra` 按次覆盖。

### 与基座中文枚举的映射

基座 `notify_channel` 表使用中文枚举（如 `微信` + `企业微信应用消息`），可用 `notify.LegacyMap(deliveryType, serviceProvider)` 转为新包的 channel/provider key，无需 DB 迁移。

## template 子包

`${var}` 变量替换 + `${= js表达式}` 求值（goja）：

```go
import "github.com/han3sui/iot-platform-notify/template"

rendered, err := template.Render(
    map[string]any{"content": "设备${name} 值=${= value.toFixed(2) }"},
    map[string]any{"name": "PV-001", "value": 3.14159},
)
// rendered["content"] == "设备PV-001 值=3.14"
```

变量支持 string/数值/bool/time.Time；表达式失败时保留原文；未定义变量保留原文。

## queue 子包

泛型 Redis Stream 队列（消费组 + 失败重投 + 死信 + 延迟 ZSET），payload 由业务自定义：

```go
import "github.com/han3sui/iot-platform-notify/queue"

q, _ := queue.New[MyPayload](rdb, queue.Options{
    StreamKey:     "biz:notify:events",
    GroupName:     "biz-notify-workers",
    DeadLetterKey: "biz:notify:deadletter",
    DelayedKey:    "biz:notify:delayed", // 空则禁用延迟能力
})
_ = q.EnsureGroup(ctx)
_, _ = q.Enqueue(ctx, &MyPayload{...})
msgs, _ := q.ReadBatch(ctx, "consumer-1", 10, 5*time.Second)
_ = q.Ack(ctx, msgs[0].ID)
// 失败重投：定时调用 ClaimStaleMessages；延迟补发：定时调用 PopDue
```

## 安全约定

- 包内不打印渠道配置内容（含密钥），错误信息只带渠道名与渠道方错误码
- 密钥的存储与加密由调用方负责
- `notify.HTTPClient` 默认 10s 超时，可在启动时整体替换

## 已知注意事项

- 腾讯云短信/邮件/语音按官方 TC3-HMAC-SHA256 v3 API 实现（基座旧代码使用已下线的 v2 接口或存在签名错误，本包已重写），**首次接入需用真实凭证验证**
- 企微/钉钉应用消息的 access_token 缓存在 Sender 实例内，请通过 `NewCached` 或自行复用实例
- 自建邮箱（gomail）不支持 ctx 取消

## 版本

当前 `v0.x`，接口可能调整；基座与微电网均稳定接入后发布 `v1.0.0`。
