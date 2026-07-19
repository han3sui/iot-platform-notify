package wechat_test

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
	_ "github.com/han3sui/iot-platform-notify/channel/wechat"
)

func TestRobotSend(t *testing.T) {
	var gotBody string
	channeltest.Intercept(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		fmt.Fprint(w, `{"errcode":0,"errmsg":"ok"}`)
	})

	s, err := notify.New(notify.ChannelWeChat, notify.ProviderRobot,
		json.RawMessage(`{"webhookUrl":"https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=test"}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Send(context.Background(), &notify.SendRequest{Content: "hello"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotBody, `"content":"hello"`) {
		t.Errorf("body = %s", gotBody)
	}
}

func TestRobotSendError(t *testing.T) {
	channeltest.Intercept(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"errcode":93000,"errmsg":"invalid webhook url"}`)
	})

	s, _ := notify.New(notify.ChannelWeChat, notify.ProviderRobot,
		json.RawMessage(`{"webhookUrl":"https://qyapi.weixin.qq.com/x"}`))
	_, err := s.Send(context.Background(), &notify.SendRequest{Content: "hi"})
	if !errors.Is(err, notify.ErrSend) {
		t.Errorf("expected ErrSend, got %v", err)
	}
}

func TestAppSendWithToken(t *testing.T) {
	var tokenCalls, sendCalls int
	channeltest.Intercept(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "gettoken") {
			tokenCalls++
			fmt.Fprint(w, `{"errcode":0,"access_token":"tok123","expires_in":7200}`)
			return
		}
		sendCalls++
		if !strings.Contains(r.URL.RawQuery, "access_token=tok123") {
			t.Errorf("missing access_token in query: %s", r.URL.RawQuery)
		}
		b, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(b), `"touser":"user1"`) {
			t.Errorf("body = %s", string(b))
		}
		fmt.Fprint(w, `{"errcode":0,"errmsg":"ok","msgid":"m1"}`)
	})

	s, err := notify.New(notify.ChannelWeChat, notify.ProviderApp,
		json.RawMessage(`{"corpId":"c1","corpSecret":"s1","agentId":1000001}`))
	if err != nil {
		t.Fatal(err)
	}
	res, err := s.Send(context.Background(), &notify.SendRequest{To: "user1", Content: "**md**"})
	if err != nil {
		t.Fatal(err)
	}
	if res.MessageID != "m1" {
		t.Errorf("MessageID = %s", res.MessageID)
	}
	// 第二次发送应复用 token
	if _, err := s.Send(context.Background(), &notify.SendRequest{To: "user1", Content: "x"}); err != nil {
		t.Fatal(err)
	}
	if tokenCalls != 1 {
		t.Errorf("token should be cached, got %d calls", tokenCalls)
	}
	if sendCalls != 2 {
		t.Errorf("sendCalls = %d", sendCalls)
	}
}

func TestAppConfigMissing(t *testing.T) {
	_, err := notify.New(notify.ChannelWeChat, notify.ProviderApp, json.RawMessage(`{"corpId":"c1"}`))
	if !errors.Is(err, notify.ErrConfig) {
		t.Errorf("expected ErrConfig, got %v", err)
	}
}
