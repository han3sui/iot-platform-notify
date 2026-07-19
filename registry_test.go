package notify_test

import (
	"encoding/json"
	"errors"
	"testing"

	notify "github.com/han3sui/iot-platform-notify"
	_ "github.com/han3sui/iot-platform-notify/channel/all"
)

func TestRegistryAllChannels(t *testing.T) {
	// 16 组渠道全部应已通过 init 注册
	expected := [][2]string{
		{notify.ChannelEmail, notify.ProviderSelfHosted},
		{notify.ChannelEmail, notify.ProviderAliyun},
		{notify.ChannelEmail, notify.ProviderHuawei},
		{notify.ChannelEmail, notify.ProviderTencent},
		{notify.ChannelSMS, notify.ProviderAliyun},
		{notify.ChannelSMS, notify.ProviderHuawei},
		{notify.ChannelSMS, notify.ProviderTencent},
		{notify.ChannelPhone, notify.ProviderAliyun},
		{notify.ChannelPhone, notify.ProviderHuawei},
		{notify.ChannelPhone, notify.ProviderTencent},
		{notify.ChannelWeChat, notify.ProviderApp},
		{notify.ChannelWeChat, notify.ProviderRobot},
		{notify.ChannelDingTalk, notify.ProviderApp},
		{notify.ChannelDingTalk, notify.ProviderRobot},
		{notify.ChannelTelegram, notify.ProviderBot},
		{notify.ChannelWebhook, notify.ProviderDefault},
	}
	registered := map[string]bool{}
	for _, k := range notify.Channels() {
		registered[k] = true
	}
	for _, e := range expected {
		if !registered[e[0]+"/"+e[1]] {
			t.Errorf("channel %s/%s not registered", e[0], e[1])
		}
	}
	if len(registered) != len(expected) {
		t.Errorf("expected %d channels, got %d: %v", len(expected), len(registered), notify.Channels())
	}
}

func TestNewUnsupported(t *testing.T) {
	_, err := notify.New("nosuch", "provider", json.RawMessage(`{}`))
	if !errors.Is(err, notify.ErrUnsupported) {
		t.Errorf("expected ErrUnsupported, got %v", err)
	}
}

func TestNewInvalidConfig(t *testing.T) {
	// 缺少必填字段应返回 ErrConfig
	_, err := notify.New(notify.ChannelTelegram, notify.ProviderBot, json.RawMessage(`{}`))
	if !errors.Is(err, notify.ErrConfig) {
		t.Errorf("expected ErrConfig, got %v", err)
	}
}

func TestNewCachedReuse(t *testing.T) {
	cfg := json.RawMessage(`{"botToken":"tok-cache-test"}`)
	s1, err := notify.NewCached(notify.ChannelTelegram, notify.ProviderBot, cfg)
	if err != nil {
		t.Fatal(err)
	}
	s2, err := notify.NewCached(notify.ChannelTelegram, notify.ProviderBot, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if s1 != s2 {
		t.Error("NewCached should return the same instance for identical config")
	}
	// 不同配置应返回不同实例
	s3, err := notify.NewCached(notify.ChannelTelegram, notify.ProviderBot, json.RawMessage(`{"botToken":"other"}`))
	if err != nil {
		t.Fatal(err)
	}
	if s1 == s3 {
		t.Error("NewCached should return different instances for different config")
	}
}

func TestLegacyMap(t *testing.T) {
	cases := []struct {
		dt, sp, ch, pv string
	}{
		{"微信", "企业微信应用消息", notify.ChannelWeChat, notify.ProviderApp},
		{"微信", "企业微信群机器人", notify.ChannelWeChat, notify.ProviderRobot},
		{"邮件", "自建邮箱", notify.ChannelEmail, notify.ProviderSelfHosted},
		{"邮件", "阿里云邮箱", notify.ChannelEmail, notify.ProviderAliyun},
		{"邮件", "华为云邮箱", notify.ChannelEmail, notify.ProviderHuawei},
		{"邮件", "腾讯云邮箱", notify.ChannelEmail, notify.ProviderTencent},
		{"短信", "阿里云短信", notify.ChannelSMS, notify.ProviderAliyun},
		{"短信", "华为云短信", notify.ChannelSMS, notify.ProviderHuawei},
		{"短信", "腾讯云短信", notify.ChannelSMS, notify.ProviderTencent},
		{"电话", "阿里云电话", notify.ChannelPhone, notify.ProviderAliyun},
		{"电话", "华为云电话", notify.ChannelPhone, notify.ProviderHuawei},
		{"电话", "腾讯云电话", notify.ChannelPhone, notify.ProviderTencent},
		{"Telegram", "Telegram机器人", notify.ChannelTelegram, notify.ProviderBot},
	}
	for _, c := range cases {
		ch, pv, ok := notify.LegacyMap(c.dt, c.sp)
		if !ok || ch != c.ch || pv != c.pv {
			t.Errorf("LegacyMap(%s, %s) = (%s, %s, %v), want (%s, %s, true)", c.dt, c.sp, ch, pv, ok, c.ch, c.pv)
		}
	}
	if _, _, ok := notify.LegacyMap("未知", "未知"); ok {
		t.Error("LegacyMap should return false for unknown mapping")
	}
}