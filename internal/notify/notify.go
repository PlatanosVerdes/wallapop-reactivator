// Package notify sends the only messages this service produces: the ones a human has to
// act on. A quiet run says nothing.
package notify

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Telegram struct {
	Token  string
	Chat   string
	Client *http.Client
}

func NewTelegram(token, chat string) *Telegram {
	return &Telegram{Token: token, Chat: chat, Client: &http.Client{Timeout: 15 * time.Second}}
}

func (t *Telegram) Enabled() bool { return t != nil && t.Token != "" && t.Chat != "" }

func (t *Telegram) Send(ctx context.Context, text string) error {
	if !t.Enabled() {
		return nil
	}
	form := url.Values{}
	form.Set("chat_id", t.Chat)
	form.Set("text", text)
	form.Set("disable_web_page_preview", "true")

	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.Token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := t.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("telegram answered %d", resp.StatusCode)
	}
	return nil
}
