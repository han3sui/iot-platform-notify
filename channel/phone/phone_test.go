package phone_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	notify "github.com/han3sui/iot-platform-notify"
	"github.com/han3sui/iot-platform-notify/channel/channeltest"
	_ "github.com/han3sui/iot-platform-notify/channel/phone"
)

func TestAliyunCall(t *testing.T) {
	var gotQuery string
	channeltest.Intercept(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		fmt.Fprint(w, `{"Code":"OK","Message":"OK","CallId":"call1"}`)
	})

	s, err := notify.New(notify.ChannelPhone, notify.ProviderAliyun,
		json.RawMessage(`{"accessKeyId":"ak","accessKeySecret":"sk","ttsCode":"TTS_1","calledShowNumber":"057100000000"}`))
	if err != nil {
		t.Fatal(err)
	}
	res, err := s.Send(context.Background(), &notify.SendRequest{To: "13800138000", Content: "设备离线"})
	if err != nil {
		t.Fatal(err)
	}
	if res.MessageID != "call1" {
		t.Errorf("MessageID = %s", res.MessageID)
	}
	for _, key := range []string{"Action=SingleCallByTts", "CalledNumber=13800138000", "TtsCode=TTS_1"} {
		if !strings.Contains(gotQuery, key) {
			t.Errorf("query missing %s: %s", key, gotQuery)
		}
	}
}

func TestAliyunCallError(t *testing.T) {
	channeltest.Intercept(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"Code":"isv.INVALID_PARAMETERS","Message":"参数错误"}`)
	})
	s, _ := notify.New(notify.ChannelPhone, notify.ProviderAliyun,
		json.RawMessage(`{"accessKeyId":"a","accessKeySecret":"s","ttsCode":"t"}`))
	_, err := s.Send(context.Background(), &notify.SendRequest{To: "138", Content: "x"})
	if !errors.Is(err, notify.ErrSend) {
		t.Errorf("expected ErrSend, got %v", err)
	}
}

func TestHuaweiCall(t *testing.T) {
	var gotBody string
	channeltest.Intercept(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		fmt.Fprint(w, `{"resultcode":"0","resultdesc":"Success"}`)
	})

	s, err := notify.New(notify.ChannelPhone, notify.ProviderHuawei,
		json.RawMessage(`{"endpoint":"https://voice.example.com/call","appKey":"k","appSecret":"s","callFrom":"+8675500000000","templateId":"tts1"}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Send(context.Background(), &notify.SendRequest{To: "+8613800138000", Content: "告警"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotBody, `"callee":"+8613800138000"`) || !strings.Contains(gotBody, `"tts_template_id":"tts1"`) {
		t.Errorf("body = %s", gotBody)
	}
}

func TestTencentCallTC3(t *testing.T) {
	var gotBody string
	var gotHeaders http.Header
	channeltest.Intercept(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		gotHeaders = r.Header.Clone()
		fmt.Fprint(w, `{"Response":{"SendStatus":{"CallId":"c1","SessionId":"s1"}}}`)
	})

	s, err := notify.New(notify.ChannelPhone, notify.ProviderTencent,
		json.RawMessage(`{"secretId":"sid","secretKey":"skey","appId":"1400002","templateId":"tts9"}`))
	if err != nil {
		t.Fatal(err)
	}
	res, err := s.Send(context.Background(), &notify.SendRequest{To: "13800138000", Content: "设备告警"})
	if err != nil {
		t.Fatal(err)
	}
	if res.MessageID != "c1" {
		t.Errorf("MessageID = %s", res.MessageID)
	}
	// 修复验证：Action 必须是 SendTtsVoice（基座旧代码误写为 DescribeInstances）
	if gotHeaders.Get("X-TC-Action") != "SendTtsVoice" {
		t.Errorf("X-TC-Action = %s", gotHeaders.Get("X-TC-Action"))
	}
	if gotHeaders.Get("X-TC-Version") != "2020-09-02" {
		t.Errorf("X-TC-Version = %s", gotHeaders.Get("X-TC-Version"))
	}
	if !strings.HasPrefix(gotHeaders.Get("Authorization"), "TC3-HMAC-SHA256 Credential=sid/") {
		t.Errorf("Authorization = %s", gotHeaders.Get("Authorization"))
	}
	// 号码补 +86；请求体字段与 VMS SendTtsVoice 文档一致
	for _, key := range []string{`"CalledNumber":"+8613800138000"`, `"TemplateId":"tts9"`, `"VoiceSdkAppid":"1400002"`} {
		if !strings.Contains(gotBody, key) {
			t.Errorf("body missing %s: %s", key, gotBody)
		}
	}
}

func TestTencentCallError(t *testing.T) {
	channeltest.Intercept(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"Response":{"Error":{"Code":"InvalidParameterValue","Message":"bad"}}}`)
	})
	s, _ := notify.New(notify.ChannelPhone, notify.ProviderTencent,
		json.RawMessage(`{"secretId":"s","secretKey":"k","appId":"1","templateId":"t"}`))
	_, err := s.Send(context.Background(), &notify.SendRequest{To: "138", Content: "x"})
	if !errors.Is(err, notify.ErrSend) {
		t.Errorf("expected ErrSend, got %v", err)
	}
}
