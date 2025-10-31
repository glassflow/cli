package install

import (
	"context"
	"fmt"

	"github.com/glassflow/glassflow-cli/internal/config"
	"github.com/glassflow/glassflow-cli/internal/helm"
	"github.com/glassflow/glassflow-cli/internal/k8s"
)

type Manager struct {
	helmManager *helm.Manager
	k8sManager  *k8s.Manager
	config      *Config
}

type Config struct {
	Namespace string
	Demo      bool
	Charts    *config.ChartsConfig
}

type StartOptions struct {
	IncludeDemo bool
	Namespace   string
}

func NewManager(helmManager *helm.Manager, k8sManager *k8s.Manager, config *Config) *Manager {
	return &Manager{
		helmManager: helmManager,
		k8sManager:  k8sManager,
		config:      config,
	}
}

func (i *Manager) StartEnvironment(ctx context.Context, opts *StartOptions) error {
	// Add Helm repositories
	if err := i.addRepositories(ctx); err != nil {
		return fmt.Errorf("failed to add repositories: %w", err)
	}

	// Install GlassFlow chart
	if err := i.installGlassFlow(ctx); err != nil {
		return fmt.Errorf("failed to install GlassFlow: %w", err)
	}

	// Install demo services if requested
	if opts.IncludeDemo {
		if err := i.installKafka(ctx); err != nil {
			return fmt.Errorf("failed to install Kafka: %w", err)
		}

		if err := i.installClickHouse(ctx); err != nil {
			return fmt.Errorf("failed to install ClickHouse: %w", err)
		}

		if err := i.setupDemo(ctx); err != nil {
			return fmt.Errorf("failed to setup demo: %w", err)
		}
	}

	return nil
}

func (i *Manager) StopEnvironment(ctx context.Context) error {
	// Uninstall GlassFlow chart
	if err := i.helmManager.UninstallChart(ctx, &helm.UninstallOptions{
		ReleaseName: "glassflow",
		Namespace:   i.config.Namespace,
		Wait:        true,
	}); err != nil {
		// Log error but continue with cleanup
		fmt.Printf("Warning: failed to uninstall GlassFlow: %v\n", err)
	}

	// Uninstall demo services (ignore if not found, we have force delete cluster option)
	if i.config.Demo {
		if err := i.helmManager.UninstallChart(ctx, &helm.UninstallOptions{
			ReleaseName: "kafka",
			Namespace:   i.config.Namespace,
			Wait:        true,
		}); err != nil {
			fmt.Printf("Warning: failed to uninstall Kafka: %v\n", err)
		}

		if err := i.helmManager.UninstallChart(ctx, &helm.UninstallOptions{
			ReleaseName: "clickhouse",
			Namespace:   i.config.Namespace,
			Wait:        true,
		}); err != nil {
			fmt.Printf("Warning: failed to uninstall ClickHouse: %v\n", err)
		}
	}

	// Delete Kind cluster
	if err := i.k8sManager.DeleteCluster(ctx); err != nil {
		return fmt.Errorf("failed to delete Kind cluster: %w", err)
	}

	return nil
}

func (i *Manager) addRepositories(ctx context.Context) error {
	repositories := []helm.Repository{
		{Name: "glassflow", URL: i.config.Charts.GlassFlow.Repository},
		{Name: "bitnami", URL: i.config.Charts.Kafka.Repository}, // TODO - use stable repos instead of bitnamilegacy
	}

	for _, repo := range repositories {
		if err := i.helmManager.AddRepository(ctx, &repo); err != nil {
			return fmt.Errorf("failed to add repository %s: %w", repo.Name, err)
		}
	}

	return nil
}

func (i *Manager) installGlassFlow(ctx context.Context) error {
	// Use default values for GlassFlow chart
	values := map[string]interface{}{}

	_, err := i.helmManager.InstallChart(ctx, &helm.InstallOptions{
		Chart:           i.config.Charts.GlassFlow.Chart,
		ReleaseName:     "glassflow",
		Namespace:       i.config.Namespace,
		Values:          values,
		CreateNamespace: true,
		Wait:            true,
		Timeout:         600, // 10 minutes timeout
	})

	return err
}

func (i *Manager) installKafka(ctx context.Context) error {
	// Use config values for images and disable persistence for local dev
	values := map[string]interface{}{
		"persistence": map[string]interface{}{
			"enabled": false, // Use emptyDir for local dev
		},
		"image": map[string]interface{}{
			"registry":   i.config.Charts.Kafka.Image.Registry,
			"repository": i.config.Charts.Kafka.Image.Repository,
		},
		"zookeeper": map[string]interface{}{
			"image": map[string]interface{}{
				"registry":   i.config.Charts.Kafka.Zookeeper.Registry,
				"repository": i.config.Charts.Kafka.Zookeeper.Repository,
			},
		},
	}

	_, err := i.helmManager.InstallChart(ctx, &helm.InstallOptions{
		Chart:           i.config.Charts.Kafka.Chart,
		ReleaseName:     "kafka",
		Namespace:       i.config.Namespace,
		Values:          values,
		CreateNamespace: true,
		Wait:            true,
		Timeout:         300,
	})

	return err
}

func (i *Manager) installClickHouse(ctx context.Context) error {
	// Use config values for images and disable persistence for local dev
	values := map[string]interface{}{
		"persistence": map[string]interface{}{
			"enabled": false, // Use emptyDir for local dev
		},
		"image": map[string]interface{}{
			"registry":   i.config.Charts.ClickHouse.Image.Registry,
			"repository": i.config.Charts.ClickHouse.Image.Repository,
		},
		"keeper": map[string]interface{}{
			"image": map[string]interface{}{
				"registry":   i.config.Charts.ClickHouse.Keeper.Registry,
				"repository": i.config.Charts.ClickHouse.Keeper.Repository,
			},
		},
	}

	_, err := i.helmManager.InstallChart(ctx, &helm.InstallOptions{
		Chart:           i.config.Charts.ClickHouse.Chart,
		ReleaseName:     "clickhouse",
		Namespace:       i.config.Namespace,
		Values:          values,
		CreateNamespace: true,
		Wait:            true,
		Timeout:         300,
	})

	return err
}

func (i *Manager) setupDemo(ctx context.Context) error {
	// TODO: Implement demo data setup
	// - Create sample Kafka topics
	// - Generate sample data
	// - Create sample pipeline configuration
	// - Start sample pipeline
	return nil
}
