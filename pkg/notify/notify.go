// Package notify sends deployment/event notifications to external channels
// (Slack, Discord, Telegram, generic webhook, email).
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp"
	"time"
)

// Level classifies an event.
type Level string

const (
	Info    Level = "info"
	Success Level = "success"
	Error   Level = "error"
)

// Event is a notification payload.
type Event struct {
	Title   string `json:"title"`
	Message string `json:"message"`
	Level   Level  `json:"level"`
	Project string `json:"project,omitempty"`
	App     string `json:"app,omitempty"`
}

// Channel is a configured destination.
type Channel struct {
	Type   string            // slack | discord | telegram | webhook | email
	Config map[string]string // provider-specific fields
}

var httpClient = &http.Client{Timeout: 15 * time.Second}

// Send delivers an event to a channel.
func Send(ctx context.Context, ch Channel, ev Event) error {
	switch ch.Type {
	case "slack":
		return postJSON(ctx, ch.Config["url"], map[string]string{"text": render(ev)})
	case "discord":
		return postJSON(ctx, ch.Config["url"], map[string]string{"content": render(ev)})
	case "telegram":
		url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", ch.Config["token"])
		return postJSON(ctx, url, map[string]string{"chat_id": ch.Config["chatId"], "text": render(ev)})
	case "webhook":
		return postJSON(ctx, ch.Config["url"], ev)
	case "email":
		return sendEmail(ch.Config, ev)
	default:
		return fmt.Errorf("unknown channel type %q", ch.Type)
	}
}

func render(ev Event) string {
	icon := "ℹ️"
	switch ev.Level {
	case Success:
		icon = "✅"
	case Error:
		icon = "❌"
	}
	s := fmt.Sprintf("%s *%s*", icon, ev.Title)
	if ev.Project != "" {
		s += fmt.Sprintf(" · %s", ev.Project)
	}
	if ev.Message != "" {
		s += "\n" + ev.Message
	}
	return s
}

func postJSON(ctx context.Context, url string, body any) error {
	if url == "" {
		return fmt.Errorf("missing url")
	}
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("notify %s: status %d", url, resp.StatusCode)
	}
	return nil
}

func sendEmail(cfg map[string]string, ev Event) error {
	host := cfg["host"]
	if host == "" {
		return fmt.Errorf("missing smtp host")
	}
	addr := host + ":" + orDefault(cfg["port"], "587")
	auth := smtp.PlainAuth("", cfg["username"], cfg["password"], host)
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: [Kapibara] %s\r\n\r\n%s\r\n",
		cfg["from"], cfg["to"], ev.Title, ev.Message)
	return smtp.SendMail(addr, auth, cfg["from"], []string{cfg["to"]}, []byte(msg))
}

func orDefault(v, def string) string {
	if v != "" {
		return v
	}
	return def
}
