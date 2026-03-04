package demo

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

//go:embed demo_pipeline_request.json
var defaultPipelineRequestJSON []byte

// version is set by SetVersion and included in demo API request headers.
var version string = "dev"

// SetVersion sets the CLI version used in demo API requests.
func SetVersion(v string) {
	version = v
}

// ConnectionError represents a connection failure that might indicate port forwarding is down
type ConnectionError struct {
	Err error
	URL string
}

func (e *ConnectionError) Error() string {
	return fmt.Sprintf("connection error to %s: %v", e.URL, e.Err)
}

func (e *ConnectionError) Unwrap() error {
	return e.Err
}

// IsConnectionError checks if an error is a ConnectionError
func IsConnectionError(err error) bool {
	var connErr *ConnectionError
	return errors.As(err, &connErr)
}

// APIClient handles communication with GlassFlow API
type APIClient struct {
	baseURL string
	client  *http.Client
}

// NewAPIClient creates a new GlassFlow API client
func NewAPIClient(baseURL string) *APIClient {
	return &APIClient{
		baseURL: baseURL,
		client:  &http.Client{},
	}
}

// PipelineHealthResponse is the response from GET /api/v1/pipeline/{id}/health
type PipelineHealthResponse struct {
	CreatedAt     string `json:"created_at"`
	OverallStatus string `json:"overall_status"`
	PipelineID    string `json:"pipeline_id"`
	PipelineName  string `json:"pipeline_name"`
	UpdatedAt     string `json:"updated_at"`
}

// GetPipelineHealth fetches the health status for a pipeline (GET /api/v1/pipeline/{id}/health).
func (c *APIClient) GetPipelineHealth(ctx context.Context, pipelineID string) (*PipelineHealthResponse, error) {
	if pipelineID == "" {
		return nil, fmt.Errorf("pipeline ID is required")
	}
	url := fmt.Sprintf("%s/api/v1/pipeline/%s/health", c.baseURL, pipelineID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "connection refused") ||
			strings.Contains(errStr, "dial tcp") ||
			strings.Contains(errStr, "no such host") ||
			strings.Contains(errStr, "timeout") ||
			strings.Contains(errStr, "connect: connection refused") {
			return nil, &ConnectionError{Err: err, URL: url}
		}
		return nil, fmt.Errorf("failed to get pipeline health: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pipeline health request failed with status %d: %s", resp.StatusCode, string(body))
	}
	var health PipelineHealthResponse
	if err := json.Unmarshal(body, &health); err != nil {
		return nil, fmt.Errorf("failed to parse pipeline health response: %w", err)
	}
	return &health, nil
}

// CreatePipeline reads the pipeline request JSON and creates a pipeline via API.
// It injects Kafka SASL authentication credentials and ClickHouse credentials before sending.
// Returns the pipeline ID from the response when successful, or empty string when pipeline already exists (403).
func (c *APIClient) CreatePipeline(ctx context.Context, requestJSONPath string, kafkaUsername, kafkaPassword, clickhouseUsername, clickhousePassword string) (string, error) {
	// Read the pipeline request JSON file
	data, err := os.ReadFile(requestJSONPath)
	if err != nil {
		return "", fmt.Errorf("failed to read pipeline request file: %w", err)
	}

	// Parse JSON to a map so we can modify it
	var requestBody map[string]interface{}
	if err := json.Unmarshal(data, &requestBody); err != nil {
		return "", fmt.Errorf("failed to parse pipeline request JSON: %w", err)
	}

	// Kafka is configured with SASL authentication
	// Set SASL authentication credentials
	if source, ok := requestBody["source"].(map[string]interface{}); ok {
		if connParams, ok := source["connection_params"].(map[string]interface{}); ok {
			// Remove skip_auth if present
			delete(connParams, "skip_auth")
			// Set SASL authentication
			connParams["protocol"] = "SASL_PLAINTEXT"
			connParams["mechanism"] = "PLAIN"
			connParams["username"] = kafkaUsername
			connParams["password"] = kafkaPassword
		}
	}

	// Inject ClickHouse credentials (password must be base64 encoded)
	if sink, ok := requestBody["sink"].(map[string]interface{}); ok {
		sink["username"] = clickhouseUsername
		// Base64 encode the password as required by GlassFlow API
		sink["password"] = base64.StdEncoding.EncodeToString([]byte(clickhousePassword))
	}

	// Re-marshal the updated JSON
	data, err = json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal updated pipeline request: %w", err)
	}

	// Create HTTP request
	url := fmt.Sprintf("%s/api/v1/pipeline", c.baseURL)
	fmt.Printf("📡 Sending pipeline creation request to: %s\n", url)
	fmt.Printf("📄 Request body size: %d bytes\n", len(data))

	// Debug: Log pipeline configuration
	debugBody := make(map[string]interface{})
	if err := json.Unmarshal(data, &debugBody); err == nil {
		if source, ok := debugBody["source"].(map[string]interface{}); ok {
			if connParams, ok := source["connection_params"].(map[string]interface{}); ok {
				if username, ok := connParams["username"].(string); ok && len(username) > 0 {
					connParams["username"] = "[REDACTED]"
				}
				if password, ok := connParams["password"].(string); ok && len(password) > 0 {
					connParams["password"] = "[REDACTED]"
				}
				fmt.Printf("🔍 Kafka connection config: protocol=%v, mechanism=%v, username=%v, password=%v\n",
					connParams["protocol"], connParams["mechanism"], connParams["username"], connParams["password"])
			}
		}
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(data))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", fmt.Sprintf("glassflow-cli/%s", version))

	// Send request
	resp, err := c.client.Do(req)
	if err != nil {
		// Check if it's a connection error (could indicate port forward is down)
		errStr := err.Error()
		isConnectionError := strings.Contains(errStr, "connection refused") ||
			strings.Contains(errStr, "dial tcp") ||
			strings.Contains(errStr, "no such host") ||
			strings.Contains(errStr, "timeout") ||
			strings.Contains(errStr, "connect: connection refused")

		if isConnectionError {
			return "", &ConnectionError{Err: err, URL: url}
		}
		return "", fmt.Errorf("failed to send request to %s: %w", url, err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	// Check status code
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		// Try to parse error response as JSON
		var errorResp map[string]interface{}
		if err := json.Unmarshal(body, &errorResp); err == nil {
			// Check if it's a "pipeline already exists" error (403)
			if resp.StatusCode == 403 {
				if msg, ok := errorResp["message"].(string); ok && (contains(msg, "already exists") || contains(msg, "duplicate")) {
					fmt.Printf("ℹ️  Pipeline already exists, continuing with producer deployment...\n")
					// Try to get pipeline_id from error response for health polling
					if id, _ := errorResp["pipeline_id"].(string); id != "" {
						return id, nil
					}
					return "", nil
				}
			}
			fmt.Printf("❌ Pipeline creation failed (status %d): %v\n", resp.StatusCode, errorResp)
			return "", fmt.Errorf("pipeline creation failed with status %d", resp.StatusCode)
		}
		// If not JSON, print raw response
		fmt.Printf("❌ Pipeline creation failed (status %d): %s\n", resp.StatusCode, string(body))
		return "", fmt.Errorf("pipeline creation failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response for pipeline_id (support "pipeline_id" or "id")
	var createResp map[string]interface{}
	if err := json.Unmarshal(body, &createResp); err == nil {
		if id, ok := createResp["pipeline_id"].(string); ok && id != "" {
			fmt.Println("✅ Pipeline created successfully")
			return id, nil
		}
		if id, ok := createResp["id"].(string); ok && id != "" {
			fmt.Println("✅ Pipeline created successfully")
			return id, nil
		}
	}
	fmt.Println("✅ Pipeline created successfully")
	return "", nil
}

// LoadPipelineRequestPath returns the path to the demo pipeline request JSON.
// It tries local paths first; if not found, uses the embedded default (no network download).
func LoadPipelineRequestPath() (string, error) {
	// Try to find demo/demo_pipeline_request.json relative to current working directory
	paths := []string{
		"demo/demo_pipeline_request.json",
		"./demo/demo_pipeline_request.json",
	}

	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			absPath, err := filepath.Abs(path)
			if err != nil {
				continue
			}
			return absPath, nil
		}
	}

	// Use embedded default (no download)
	tmp, err := os.CreateTemp("", "glassflow-demo-pipeline-*.json")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file for pipeline request: %w", err)
	}
	path := tmp.Name()
	if _, err := tmp.Write(defaultPipelineRequestJSON); err != nil {
		tmp.Close()
		os.Remove(path)
		return "", fmt.Errorf("failed to write pipeline request: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(path)
		return "", fmt.Errorf("failed to close pipeline request file: %w", err)
	}
	return path, nil
}

// GetPipelineIDFromRequest reads pipeline_id from the pipeline request JSON file.
// The demo uses a fixed pipeline_id (e.g. "demo-pipeline-qasxjh") for health polling.
func GetPipelineIDFromRequest(requestJSONPath string) (string, error) {
	data, err := os.ReadFile(requestJSONPath)
	if err != nil {
		return "", fmt.Errorf("failed to read pipeline request file: %w", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return "", fmt.Errorf("failed to parse pipeline request JSON: %w", err)
	}
	if id, ok := m["pipeline_id"].(string); ok && id != "" {
		return id, nil
	}
	return "", nil
}

func contains(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
