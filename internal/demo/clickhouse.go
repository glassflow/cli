package demo

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// ClickHouseClient handles communication with ClickHouse HTTP API
type ClickHouseClient struct {
	baseURL  string
	username string
	password string
	client   *http.Client
}

// NewClickHouseClient creates a new ClickHouse HTTP client
func NewClickHouseClient(baseURL string) *ClickHouseClient {
	return &ClickHouseClient{
		baseURL:  baseURL,
		username: "default",
		password: "",
		client:   &http.Client{},
	}
}

// NewClickHouseClientWithAuth creates a new ClickHouse HTTP client with authentication
func NewClickHouseClientWithAuth(baseURL, username, password string) *ClickHouseClient {
	return &ClickHouseClient{
		baseURL:  baseURL,
		username: username,
		password: password,
		client:   &http.Client{},
	}
}

// ExecuteQuery executes a SQL query via ClickHouse HTTP API
func (c *ClickHouseClient) ExecuteQuery(ctx context.Context, query string) error {
	// URL encode the query
	encodedQuery := url.QueryEscape(query)
	requestURL := fmt.Sprintf("%s/?query=%s", c.baseURL, encodedQuery)

	req, err := http.NewRequestWithContext(ctx, "POST", requestURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Add authentication if password is provided
	if c.password != "" {
		req.SetBasicAuth(c.username, c.password)
	}

	// Send request
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	// ClickHouse returns 200 OK even for some errors, check response body
	bodyStr := string(body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("clickhouse query failed with status %d: %s", resp.StatusCode, bodyStr)
	}

	// Check for error messages in response (ClickHouse returns errors in response body)
	if strings.Contains(bodyStr, "Exception:") || strings.Contains(bodyStr, "error") {
		fmt.Printf("⚠️  ClickHouse response: %s\n", bodyStr)
		// Still return error if it's a real exception
		if strings.Contains(bodyStr, "Exception:") {
			return fmt.Errorf("clickhouse query failed: %s", bodyStr)
		}
	}

	return nil
}

// CreateTable reads the SQL file and creates a table in ClickHouse
func (c *ClickHouseClient) CreateTable(ctx context.Context, sqlFilePath string) error {
	// Read SQL file
	sql, err := os.ReadFile(sqlFilePath)
	if err != nil {
		return fmt.Errorf("failed to read SQL file: %w", err)
	}

	sqlStr := strings.TrimSpace(string(sql))
	if sqlStr == "" {
		return fmt.Errorf("SQL file is empty")
	}

	// Execute the CREATE TABLE statement
	fmt.Println("📋 Creating ClickHouse table...")
	if err := c.ExecuteQuery(ctx, sqlStr); err != nil {
		return fmt.Errorf("failed to create table: %w", err)
	}

	fmt.Println("✅ ClickHouse table created successfully")
	return nil
}

// LoadClickHouseSQLPath returns the path to the ClickHouse SQL file
func LoadClickHouseSQLPath() (string, error) {
	// Try to find demo/clickhouse_table.sql relative to current working directory
	paths := []string{
		"demo/clickhouse_table.sql",
		"./demo/clickhouse_table.sql",
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

	return "", fmt.Errorf("clickhouse_table.sql not found in demo directory")
}
