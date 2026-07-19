package sms_test

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
	_ "github.com/han3sui/iot-platform-notify/channel/sms"
)

func TestAliyunSend(t *testing.T) {
	var gotQuery string
	channeltest.Intercept(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		fmt.Fprint(w, `{"Code":"OK","Message":"OK","BizId":"biz1"}`)
	})

	s, err := notify.New(notify.ChannelSMS, notify.ProviderAliyun,
		json.RawMessage(`{"accessKeyId":"ak","accessKeySecret":"sk","signName":"测试签名","templateCode":"SMS_123"}`))
	if err != nil {
		t.Fatal(err)
	}
	res, err := s.Send(context.Background(), &notify.SendRequest{To: "13800138000", Content: "code=1234"})
	if err != nil {
		t.Fatal(err)
	}
	if res.MessageID != "biz1" {
		t.Errorf("MessageID = %s", res.MessageID)
	}
	for _, key := range []string{"Action=SendSms", "PhoneNumbers=13800138000", "Signature="} {
		if !strings.Contains(gotQuery, key) {
			t.Errorf("query missing %s: %s", key, gotQuery)
		}
	}
}

func TestAliyunBizError(t *testing.T) {
	channeltest.Intercept(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"Code":"isv.SMS_SIGNATURE_ILLEGAL","Message":"签名不合法"}`)
	})
	s, _ := notify.New(notify.ChannelSMS, notify.ProviderAliyun,
		json.RawMessage(`{"accessKeyId":"ak","accessKeySecret":"sk","signName":"x","templateCode":"y"}`))
	_, err := s.Send(context.Background(), &notify.SendRequest{To: "138", Content: "x"})
	if !errors.Is(err, notify.ErrSend) {
		t.Errorf("expected ErrSend, got %v", err)
	}
}

func TestHuaweiSend(t *testing.T) {
	var gotBody, gotWSSE string
	channeltest.Intercept(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		gotWSSE = r.Header.Get("X-WSSE")
		fmt.Fprint(w, `{"code":"000000","description":"Success","result":[{"smsMsgId":"m1","status":"000000"}]}`)
	})

	s, err := notify.New(notify.ChannelSMS, notify.ProviderHuawei,
		json.RawMessage(`{"endpoint":"https://smsapi.cn-north-4.myhuaweicloud.com","accessKeyId":"ak","accessKeySecret":"sk","from":"csms1","templateId":"tpl1"}`))
	if err != nil {
		t.Fatal(err)
	}
	res, err := s.Send(context.Background(), &notify.SendRequest{To: "13800138000", Content: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if res.MessageID != "m1" {
		t.Errorf("MessageID = %s", res.MessageID)
	}
	if !strings.Contains(gotBody, "templateId=tpl1") || !strings.Contains(gotBody, "to=13800138000") {
		t.Errorf("body = %s", gotBody)
	}
	if !strings.Contains(gotWSSE, "UsernameToken Username=\"ak\"") {
		t.Errorf("X-WSSE = %s", gotWSSE)
	}
}

func TestHuaweiStatusFail(t *testing.T) {
	channeltest.Intercept(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"code":"000000","result":[{"smsMsgId":"m1","status":"E200028"}]}`)
	})
	s, _ := notify.New(notify.ChannelSMS, notify.ProviderHuawei,
		json.RawMessage(`{"endpoint":"https://x","accessKeyId":"a","accessKeySecret":"s","from":"f","templateId":"t"}`))
	_, err := s.Send(context.Background(), &notify.SendRequest{To: "138", Content: "x"})
	if !errors.Is(err, notify.ErrSend) {
		t.Errorf("expected ErrSend, got %v", err)
	}
}

func TestTencentSendTC3(t *testing.T) {
	var gotBody string
	var gotHeaders http.Header
	channeltest.Intercept(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		gotHeaders = r.Header.Clone()
		fmt.Fprint(w, `{"Response":{"SendStatusSet":[{"SerialNo":"sn1","Code":"Ok","Message":"send success","PhoneNumber":"+8613800138000"}]}}`)
	})

	s, err := notify.New(notify.ChannelSMS, notify.ProviderTencent,
		json.RawMessage(`{"secretId":"sid","secretKey":"skey","appId":"1400001","signName":"签名","templateId":"100001"}`))
	if err != nil {
		t.Fatal(err)
	}
	res, err := s.Send(context.Background(), &notify.SendRequest{To: "13800138000", Content: "1234"})
	if err != nil {
		t.Fatal(err)
	}
	if res.MessageID != "sn1" {
		t.Errorf("MessageID = %s", res.MessageID)
	}
	// TC3 头完整性
	if gotHeaders.Get("X-TC-Action") != "SendSms" {
		t.Errorf("X-TC-Action = %s", gotHeaders.Get("X-TC-Action"))
	}
	if gotHeaders.Get("X-TC-Version") != "2021-01-11" {
		t.Errorf("X-TC-Version = %s", gotHeaders.Get("X-TC-Version"))
	}
	if !strings.HasPrefix(gotHeaders.Get("Authorization"), "TC3-HMAC-SHA256 Credential=sid/") {
		t.Errorf("Authorization = %s", gotHeaders.Get("Authorization"))
	}
	// 号码应补 +86 前缀
	if !strings.Contains(gotBody, `"+8613800138000"`) {
		t.Errorf("body = %s", gotBody)
	}
}

func TestTencentSendFail(t *testing.T) {
	channeltest.Intercept(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"Response":{"Error":{"Code":"AuthFailure.SignatureFailure","Message":"sign error"}}}`)
	})
	s, _ := notify.New(notify.ChannelSMS, notify.ProviderTencent,
		json.RawMessage(`{"secretId":"s","secretKey":"k","appId":"1","signName":"x","templateId":"y"}`))
	_, err := s.Send(context.Background(), &notify.SendRequest{To: "138", Content: "x"})
	if !errors.Is(err, notify.ErrSend) {
		t.Errorf("expected ErrSend, got %v", err)
	}
}
