package cmd

import (
	"fmt"
	"strings"

	"github.com/glassflow/glassflow-cli/internal/config"
)

func resolveKubeContext(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	if strings.TrimSpace(cfg.Context) != "" {
		return cfg.Context
	}
	if strings.TrimSpace(cfg.KindClusterName) == "" {
		return ""
	}
	return fmt.Sprintf("kind-%s", cfg.KindClusterName)
}
