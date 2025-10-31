package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

type Config struct {
	Kubeconfig string `mapstructure:"kubeconfig"`
	Context    string `mapstructure:"context"`
	Namespace  string `mapstructure:"namespace"`

	KindClusterName string `mapstructure:"kind_cluster_name"`

	Charts ChartsConfig `mapstructure:"charts"`
}

type ChartsConfig struct {
	GlassFlow  ChartConfig `mapstructure:"glassflow"`
	Kafka      ChartConfig `mapstructure:"kafka"`
	ClickHouse ChartConfig `mapstructure:"clickhouse"`
}

type ChartConfig struct {
	Repository string       `mapstructure:"repository"`
	Chart      string       `mapstructure:"chart"`
	Version    string       `mapstructure:"version"`
	Image      ImageConfig  `mapstructure:"image"`
	Zookeeper  *ImageConfig `mapstructure:"zookeeper,omitempty"`
	Keeper     *ImageConfig `mapstructure:"keeper,omitempty"`
}

type ImageConfig struct {
	Registry   string `mapstructure:"registry"`
	Repository string `mapstructure:"repository"`
}

func Load(configPath string) (*Config, error) {
	// Configure Viper based on explicit path or fallback to repo-level config.yaml
	if configPath != "" {
		viper.SetConfigFile(configPath)
	} else {
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
		viper.AddConfigPath(".")
	}

	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	if config.Kubeconfig == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		config.Kubeconfig = filepath.Join(home, ".kube", "config")
	}

	return &config, nil
}
