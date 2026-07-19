package dingtalk_test

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
	_ "github.com/han3sui/iot-platform-notify/channel/dingtalk"
)

func TestRobotSendWithSign(t *testing.T) {
	var gotQuery string
	channeltest.Intercept(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		fmt.Fprint(w, `{"errcode":0,"errmsg":"ok"}`)
	})

	s, err := notify.New(notify.ChannelDingTalk, notify.ProviderRobot,
		json.RawMessage(`{"webhookUrl":"https://oapi.dingtalk.com/robot/send?access_token=tok","secret":"SECabc"}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Send(context.Background(), &notify.SendRequest{Content: "警报"}); err != nil {
		t.Fatal(err)
	}
	// 加签模式应附带 timestamp 和 sign
	if !strings.Contains(gotQuery, "timestamp=") || !strings.Contains(gotQuery, "sign=") {
		t.Errorf("query missing signature params: %s", gotQuery)
	}
}

func TestRobotSendNoSign(t *testing.T) {
	var gotQuery string
	channeltest.Intercept(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		fmt.Fprint(w, `{"errcode":0}`)
	})

	s, _ := notify.New(notify.ChannelDingTalk, notify.ProviderRobot,
		json.RawMessage(`{"webhookUrl":"https://oapi.dingtalk.com/robot/send?access_token=tok"}`))
	if _, err := s.Send(context.Background(), &notify.SendRequest{Content: "x"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(gotQuery, "sign=") {
		t.Errorf("unsigned mode should not append sign: %s", gotQuery)
	}
}

func TestAppSend(t *testing.T) {
	channeltest.Intercept(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "gettoken") {
			fmt.Fprint(w, `{"errcode":0,"access_token":"dtok","expires_in":7200}`)
			return
		}
		b, _ := io.ReadAll(r.Body)
		body := string(b)
		if !strings.Contains(body, "userid_list=u1") {
			t.Errorf("body = %s", body)
		}
		fmt.Fprint(w, `{"errcode":0,"task_id":99}`)
	})

	s, err := notify.New(notify.ChannelDingTalk, notify.ProviderApp,
		json.RawMessage(`{"appKey":"k","appSecret":"s","agentId":123}`))
	if err != nil {
		t.Fatal(err)
	}
	res, err := s.Send(context.Background(), &notify.SendRequest{To: "u1", Content: "msg"})
	if err != nil {
		t.Fatal(err)
	}
	if res.MessageID != "99" {
		t.Errorf("MessageID = %s", res.MessageID)
	}
}

func TestRobotError(t *testing.T) {
	channeltest.Intercept(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"errcode":310000,"errmsg":"keywords not in content"}`)
	})
	s, _ := notify.New(notify.ChannelDingTalk, notify.ProviderRobot,
		json.RawMessage(`{"webhookUrl":"https://oapi.dingtalk.com/robot/send?access_token=t"}`))
	_, err := s.Send(context.Background(), &notify.SendRequest{Content: "x"})
	if !errors.Is(err, notify.ErrSend) {
		t.Errorf("expected ErrSend, got %v", err)
	}
}
