package tracking

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	baseURL   = "https://tracking.glassflow.dev/api/v1"
	username  = "5PR3vzYGGlYttkBXRTYuzw=="
	password  = "5tJcYQNR85KKyOwpeIFVdGqGIOgFwxmWEHu0t7WxBbo="
	sourceCLI = "glassflow-cli"
)

// Event names
const (
	EventUpStarted          = "cli_up_started"
	EventUpCompleted        = "cli_up_completed"
	EventUpFailed           = "cli_up_failed"
	EventSetupDemoStarted   = "cli_setup_demo_started"
	EventSetupDemoCompleted = "cli_setup_demo_completed"
	EventSetupDemoFailed    = "cli_setup_demo_failed"
	EventDownCompleted      = "cli_down_completed"
	EventDownFailed         = "cli_down_failed"
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

// cachedInstallationID holds the resolved ID for the lifetime of the process.
var cachedInstallationID string

// getInstallationID returns a persistent installation ID stored in
// ~/.glassflow/installation_id. Generated once on first call, then reused.
func getInstallationID() string {
	if cachedInstallationID != "" {
		return cachedInstallationID
	}
	home, err := os.UserHomeDir()
	if err != nil {
		cachedInstallationID = uuid.New().String()
		return cachedInstallationID
	}
	dir := filepath.Join(home, ".glassflow")
	idPath := filepath.Join(dir, "installation_id")

	data, err := os.ReadFile(idPath)
	if err == nil {
		id := strings.TrimSpace(string(data))
		if id != "" {
			cachedInstallationID = id
			return cachedInstallationID
		}
	}

	cachedInstallationID = uuid.New().String()
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(idPath, []byte(cachedInstallationID), 0o644)
	return cachedInstallationID
}

// GetInstallationID returns the persistent installation ID (for passing to Helm, etc).
func GetInstallationID() string {
	return getInstallationID()
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

func track(eventName string, properties map[string]interface{}) {
	token := getToken()
	if token == "" {
		return
	}
	body := trackRequest{
		InstallationID: getInstallationID(),
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

func truncateError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if len(msg) > 500 {
		return msg[:500] + "..."
	}
	return msg
}

// TrackUpStarted sends cli_up_started.
func TrackUpStarted(cliVersion string, demo bool) {
	track(EventUpStarted, map[string]interface{}{
		"demo":    demo,
		"version": cliVersion,
	})
}

// TrackUpCompleted sends cli_up_completed with duration.
func TrackUpCompleted(cliVersion string, demo bool, duration time.Duration) {
	track(EventUpCompleted, map[string]interface{}{
		"demo":             demo,
		"version":          cliVersion,
		"duration_seconds": int64(duration.Seconds()),
	})
}

// TrackUpFailed sends cli_up_failed with error and duration.
func TrackUpFailed(cliVersion string, demo bool, err error, duration time.Duration) {
	track(EventUpFailed, map[string]interface{}{
		"demo":             demo,
		"version":          cliVersion,
		"error":            truncateError(err),
		"duration_seconds": int64(duration.Seconds()),
	})
}

// TrackSetupDemoStarted sends cli_setup_demo_started.
func TrackSetupDemoStarted(cliVersion string) {
	track(EventSetupDemoStarted, map[string]interface{}{
		"version": cliVersion,
	})
}

// TrackSetupDemoCompleted sends cli_setup_demo_completed with duration.
func TrackSetupDemoCompleted(cliVersion string, duration time.Duration) {
	track(EventSetupDemoCompleted, map[string]interface{}{
		"version":          cliVersion,
		"duration_seconds": int64(duration.Seconds()),
	})
}

// TrackSetupDemoFailed sends cli_setup_demo_failed with error and duration.
func TrackSetupDemoFailed(cliVersion string, err error, duration time.Duration) {
	track(EventSetupDemoFailed, map[string]interface{}{
		"version":          cliVersion,
		"error":            truncateError(err),
		"duration_seconds": int64(duration.Seconds()),
	})
}

// TrackDownCompleted sends cli_down_completed.
func TrackDownCompleted(cliVersion string, force bool, duration time.Duration) {
	track(EventDownCompleted, map[string]interface{}{
		"version":          cliVersion,
		"force":            force,
		"duration_seconds": int64(duration.Seconds()),
	})
}

// TrackDownFailed sends cli_down_failed.
func TrackDownFailed(cliVersion string, force bool, err error, duration time.Duration) {
	track(EventDownFailed, map[string]interface{}{
		"version":          cliVersion,
		"force":            force,
		"error":            truncateError(err),
		"duration_seconds": int64(duration.Seconds()),
	})
}
