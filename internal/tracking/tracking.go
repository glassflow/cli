package tracking

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	baseURL   = "https://tracking.glassflow.dev/api/v1"
	username  = "5PR3vzYGGlYttkBXRTYuzw=="
	password  = "5tJcYQNR85KKyOwpeIFVdGqGIOgFwxmWEHu0t7WxBbo="
	sourceCLI = "glassflow-cli"
)

// Event names for glassflow up
const (
	EventUpStarted   = "cli_up_started"
	EventUpCompleted = "cli_up_completed"
	EventUpFailed    = "cli_up_failed"
)

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

type trackRequest struct {
	InstallationID string                 `json:"installation_id"`
	EventName      string                 `json:"event_name"`
	EventSource    string                 `json:"event_source"`
	Timestamp      string                 `json:"timestamp"`
	Properties     map[string]interface{} `json:"properties,omitempty"`
}

var cachedToken string
var tokenExpiry time.Time

func getToken() string {
	if cachedToken != "" && time.Now().Before(tokenExpiry) {
		return cachedToken
	}
	form := url.Values{}
	form.Set("username", username)
	form.Set("password", password)
	form.Set("grant_type", "password")
	req, err := http.NewRequest(http.MethodPost, baseURL+"/auth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return ""
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return ""
	}
	defer resp.Body.Close()
	var tr tokenResponse
	if json.NewDecoder(resp.Body).Decode(&tr) != nil {
		return ""
	}
	cachedToken = tr.AccessToken
	if tr.ExpiresIn > 60 {
		tokenExpiry = time.Now().Add(time.Duration(tr.ExpiresIn-60) * time.Second)
	} else {
		tokenExpiry = time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second / 2)
	}
	return cachedToken
}

func track(installationID, eventName string, properties map[string]interface{}) {
	token := getToken()
	if token == "" {
		return
	}
	body := trackRequest{
		InstallationID: installationID,
		EventName:      eventName,
		EventSource:    sourceCLI,
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		Properties:     properties,
	}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, baseURL+"/track", bytes.NewReader(raw))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return
	}
	resp.Body.Close()
}

// TrackUpStarted sends up_started. installationID is the session UUID for this run.
func TrackUpStarted(installationID, cliVersion string, demo bool) {
	track(installationID, EventUpStarted, map[string]interface{}{
		"demo":    demo,
		"version": cliVersion,
	})
}

// TrackUpCompleted sends up_completed with duration_seconds.
func TrackUpCompleted(installationID, cliVersion string, demo bool, duration time.Duration) {
	track(installationID, EventUpCompleted, map[string]interface{}{
		"demo":             demo,
		"version":          cliVersion,
		"duration_seconds": int64(duration.Seconds()),
	})
}

// TrackUpFailed sends up_failed. Error message is truncated to avoid huge payloads.
func TrackUpFailed(installationID, cliVersion string, demo bool, err error) {
	msg := ""
	if err != nil {
		msg = err.Error()
		if len(msg) > 500 {
			msg = msg[:500] + "..."
		}
	}
	track(installationID, EventUpFailed, map[string]interface{}{
		"demo":    demo,
		"version": cliVersion,
		"error":   msg,
	})
}
