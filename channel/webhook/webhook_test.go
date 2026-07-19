package webhook_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	notify "github.com/han3sui/iot-platform-notify"
	"github.com/han3sui/iot-platform-notify/channel/channeltest"
	_ "github.com/han3sui/iot-platform-notify/channel/webhook"
)

func TestDefaultSend(t *testing.T) {
	var gotBody, gotHeader string
	channeltest.Intercept(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		gotHeader = r.Header.Get("X-Token")
		w.WriteHeader(http.StatusOK)
	})

	s, err := notify.New(notify.ChannelWebhook, notify.ProviderDefault,
		json.RawMessage(`{"url":"https://hook.example.com/x","headers":{"X-Token":"tk1"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Send(context.Background(), &notify.SendRequest{Content: "payload"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotBody, `"message":"payload"`) {
		t.Errorf("body = %s", gotBody)
	}
	if gotHeader != "tk1" {
		t.Errorf("X-Token = %s", gotHeader)
	}
}

func TestDefaultSendError(t *testing.T) {
	channeltest.Intercept(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	s, _ := notify.New(notify.ChannelWebhook, notify.ProviderDefault, json.RawMessage(`{"url":"https://h/x"}`))
	_, err := s.Send(context.Background(), &notify.SendRequest{Content: "x"})
	if !errors.Is(err, notify.ErrSend) {
		t.Errorf("expected ErrSend, got %v", err)
	}
}
