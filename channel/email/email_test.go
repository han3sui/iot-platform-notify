package email_test

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
	_ "github.com/han3sui/iot-platform-notify/channel/email"
)

func TestAliyunSend(t *testing.T) {
	var gotQuery string
	channeltest.Intercept(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		fmt.Fprint(w, `{"EnvId":"e1","RequestId":"r1"}`)
	})

	s, err := notify.New(notify.ChannelEmail, notify.ProviderAliyun,
		json.RawMessage(`{"accessKeyId":"ak","accessKeySecret":"sk","accountName":"noreply@mail.example.com","fromName":"平台"}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Send(context.Background(), &notify.SendRequest{To: "a@b.com", Subject: "告警", Content: "<b>hi</b>"}); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"Action=SingleSendMail", "ToAddress=a%40b.com", "Signature="} {
		if !strings.Contains(gotQuery, key) {
			t.Errorf("query missing %s: %s", key, gotQuery)
		}
	}
}

func TestHuaweiSend(t *testing.T) {
	var gotBody, gotWSSE string
	channeltest.Intercept(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		gotWSSE = r.Header.Get("X-WSSE")
		fmt.Fprint(w, `{"result":"ok"}`)
	})

	s, err := notify.New(notify.ChannelEmail, notify.ProviderHuawei,
		json.RawMessage(`{"endpoint":"https://mail.example.com/send","appKey":"k","appSecret":"s","fromEmail":"noreply@example.com"}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Send(context.Background(), &notify.SendRequest{To: "a@b.com", Subject: "sub", Content: "<p>x</p>"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotBody, `"to":["a@b.com"]`) || !strings.Contains(gotBody, `"subject":"sub"`) {
		t.Errorf("body = %s", gotBody)
	}
	if !strings.Contains(gotWSSE, "UsernameToken Username=\"k\"") {
		t.Errorf("X-WSSE = %s", gotWSSE)
	}
}

func TestTencentSendTC3(t *testing.T) {
	var gotBody string
	var gotHeaders http.Header
	channeltest.Intercept(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		gotHeaders = r.Header.Clone()
		fmt.Fprint(w, `{"Response":{"MessageId":"mid1"}}`)
	})

	s, err := notify.New(notify.ChannelEmail, notify.ProviderTencent,
		json.RawMessage(`{"secretId":"sid","secretKey":"skey","fromEmail":"noreply@qq.example.com"}`))
	if err != nil {
		t.Fatal(err)
	}
	res, err := s.Send(context.Background(), &notify.SendRequest{To: "a@b.com", Subject: "sub", Content: "<p>x</p>"})
	if err != nil {
		t.Fatal(err)
	}
	if res.MessageID != "mid1" {
		t.Errorf("MessageID = %s", res.MessageID)
	}
	if gotHeaders.Get("X-TC-Action") != "SendEmail" {
		t.Errorf("X-TC-Action = %s", gotHeaders.Get("X-TC-Action"))
	}
	if !strings.Contains(gotBody, `"Destination":["a@b.com"]`) {
		t.Errorf("body = %s", gotBody)
	}
}

func TestTencentSendError(t *testing.T) {
	channeltest.Intercept(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"Response":{"Error":{"Code":"AuthFailure","Message":"bad sign"}}}`)
	})
	s, _ := notify.New(notify.ChannelEmail, notify.ProviderTencent,
		json.RawMessage(`{"secretId":"s","secretKey":"k","fromEmail":"x@y.com"}`))
	_, err := s.Send(context.Background(), &notify.SendRequest{To: "a@b.com", Subject: "s", Content: "c"})
	if !errors.Is(err, notify.ErrSend) {
		t.Errorf("expected ErrSend, got %v", err)
	}
}

func TestSelfHostedConfigMissing(t *testing.T) {
	_, err := notify.New(notify.ChannelEmail, notify.ProviderSelfHosted, json.RawMessage(`{"port":465}`))
	if !errors.Is(err, notify.ErrConfig) {
		t.Errorf("expected ErrConfig, got %v", err)
	}
}
