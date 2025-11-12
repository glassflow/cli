package demo

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

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

// CreatePipeline reads the pipeline request JSON and creates a pipeline via API
// It injects Kafka SASL authentication credentials and ClickHouse credentials before sending
func (c *APIClient) CreatePipeline(ctx context.Context, requestJSONPath string, kafkaUsername, kafkaPassword, clickhouseUsername, clickhousePassword string) error {
	// Read the pipeline request JSON file
	data, err := os.ReadFile(requestJSONPath)
	if err != nil {
		return fmt.Errorf("failed to read pipeline request file: %w", err)
	}

	// Parse JSON to a map so we can modify it
	var requestBody map[string]interface{}
	if err := json.Unmarshal(data, &requestBody); err != nil {
		return fmt.Errorf("failed to parse pipeline request JSON: %w", err)
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
		return fmt.Errorf("failed to marshal updated pipeline request: %w", err)
	}

	// Create HTTP request
	url := fmt.Sprintf("%s/api/v1/pipeline", c.baseURL)
	fmt.Printf("📡 Sending pipeline creation request to: %s\n", url)
	fmt.Printf("📄 Request body size: %d bytes\n", len(data))

	// Debug: Log pipeline configuration
	debugBody := make(map[string]interface{})
	json.Unmarshal(data, &debugBody) // Ignore error, this is just for logging
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

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Send request
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request to %s: %w", url, err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
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
					return nil // Treat as success, pipeline exists
				}
			}
			fmt.Printf("❌ Pipeline creation failed (status %d): %v\n", resp.StatusCode, errorResp)
			return fmt.Errorf("pipeline creation failed with status %d", resp.StatusCode)
		}
		// If not JSON, print raw response
		fmt.Printf("❌ Pipeline creation failed (status %d): %s\n", resp.StatusCode, string(body))
		return fmt.Errorf("pipeline creation failed with status %d: %s", resp.StatusCode, string(body))
	}

	fmt.Println("✅ Pipeline created successfully")
	return nil
}

// LoadPipelineRequestPath returns the path to the demo pipeline request JSON
func LoadPipelineRequestPath() (string, error) {
	// Try to find demo/demo_pipeline_request.json relative to current working directory
	// or in common locations
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

	return "", fmt.Errorf("demo_pipeline_request.json not found in demo directory")
}

func contains(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
