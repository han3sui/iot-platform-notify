package telegram_test

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
	_ "github.com/han3sui/iot-platform-notify/channel/telegram"
)

func TestBotSend(t *testing.T) {
	var gotBody, gotPath string
	channeltest.Intercept(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		fmt.Fprint(w, `{"ok":true}`)
	})

	s, err := notify.New(notify.ChannelTelegram, notify.ProviderBot,
		json.RawMessage(`{"botToken":"123:abc"}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Send(context.Background(), &notify.SendRequest{To: "-100123", Content: "alert"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotPath, "bot123:abc/sendMessage") {
		t.Errorf("path = %s", gotPath)
	}
	if !strings.Contains(gotBody, `"chat_id":"-100123"`) {
		t.Errorf("body = %s", gotBody)
	}
}

func TestBotSendError(t *testing.T) {
	channeltest.Intercept(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"ok":false,"description":"chat not found"}`)
	})
	s, _ := notify.New(notify.ChannelTelegram, notify.ProviderBot, json.RawMessage(`{"botToken":"t"}`))
	_, err := s.Send(context.Background(), &notify.SendRequest{To: "1", Content: "x"})
	if !errors.Is(err, notify.ErrSend) {
		t.Errorf("expected ErrSend, got %v", err)
	}
}
