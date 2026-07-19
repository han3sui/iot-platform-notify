// Package all 通过 blank import 一次性注册全部渠道 adapter。
//
// 用法：
//
//	import _ "github.com/han3sui/iot-platform-notify/channel/all"
package all

import (
	_ "github.com/han3sui/iot-platform-notify/channel/dingtalk"
	_ "github.com/han3sui/iot-platform-notify/channel/email"
	_ "github.com/han3sui/iot-platform-notify/channel/phone"
	_ "github.com/han3sui/iot-platform-notify/channel/sms"
	_ "github.com/han3sui/iot-platform-notify/channel/telegram"
	_ "github.com/han3sui/iot-platform-notify/channel/webhook"
	_ "github.com/han3sui/iot-platform-notify/channel/wechat"
)
