package github

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	// GitHubRepo is the GitHub repository URL
	GitHubRepo = "https://github.com/glassflow/cli"
	// GitHubRawBase is the base URL for raw GitHub content
	GitHubRawBase = "https://raw.githubusercontent.com/glassflow/cli"
)

// DownloadFile downloads a file from GitHub and saves it to a temporary file
// Returns the path to the temporary file
func DownloadFile(version, filePath string) (string, error) {
	// Determine the ref to use (tag or branch)
	// GoReleaser sets {{.Version}} as the tag without 'v' prefix (e.g., tag "v2.1.0" -> version "2.1.0")
	ref := version
	if ref == "dev" || ref == "unknown" || ref == "" {
		// For dev builds, use main branch
		ref = "main"
	} else if len(ref) > 0 && ref[0] != 'v' {
		// If version doesn't start with 'v', prepend it (e.g., "2.1.0" -> "v2.1.0")
		// This handles GoReleaser's {{.Version}} which strips the 'v' prefix
		ref = "v" + ref
	}
	// If version already starts with 'v', use it as-is

	// Construct the URL
	url := fmt.Sprintf("%s/%s/%s", GitHubRawBase, ref, filePath)

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Download the file
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to download %s from GitHub: %w", filePath, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download %s: HTTP %d", url, resp.StatusCode)
	}

	// Create temporary file
	tmpFile, err := os.CreateTemp("", filepath.Base(filePath)+"_*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}

	// Write downloaded content to temp file
	_, err = io.Copy(tmpFile, resp.Body)
	if err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("failed to write downloaded content: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("failed to close temp file: %w", err)
	}

	return tmpFile.Name(), nil
}

// DownloadConfig downloads config.yaml from GitHub
func DownloadConfig(version string) (string, error) {
	return DownloadFile(version, "config.yaml")
}

// DownloadPipelineRequest downloads demo_pipeline_request.json from GitHub
func DownloadPipelineRequest(version string) (string, error) {
	return DownloadFile(version, "demo/demo_pipeline_request.json")
}

// DownloadClickHouseSQL downloads clickhouse_table.sql from GitHub
func DownloadClickHouseSQL(version string) (string, error) {
	return DownloadFile(version, "demo/clickhouse_table.sql")
}
