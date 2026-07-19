// examples/basic 演示新业务系统接入通知包的最小用法。
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	notify "github.com/han3sui/iot-platform-notify"
	"github.com/han3sui/iot-platform-notify/template"

	// blank import 注册所需渠道（也可 import channel/all 全量注册）
	_ "github.com/han3sui/iot-platform-notify/channel/wechat"
)

func main() {
	ctx := context.Background()

	// 1. 模板渲染：模板与变量由业务系统自己定义
	tpl := map[string]any{
		"content": "【${severity}】${stationName}\n规则：${ruleName}\n触发值：${= value.toFixed(2) } kW",
	}
	data := map[string]any{
		"severity":    "严重",
		"stationName": "XX光伏电站",
		"ruleName":    "逆变器功率过低",
		"value":       3.14159,
	}
	rendered, err := template.Render(tpl, data)
	if err != nil {
		log.Fatal(err)
	}

	// 2. 渠道配置来自业务系统数据库（此处演示硬编码）
	channelConfig := json.RawMessage(`{"webhookUrl":"https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=YOUR_KEY"}`)

	// 3. 构造 Sender 并发送（带 token 状态的渠道建议用 NewCached）
	sender, err := notify.New(notify.ChannelWeChat, notify.ProviderRobot, channelConfig)
	if err != nil {
		log.Fatal(err)
	}
	result, err := sender.Send(ctx, &notify.SendRequest{
		Content: rendered["content"].(string),
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("发送成功: %+v\n", result)
}
