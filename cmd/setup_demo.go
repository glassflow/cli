package cmd

import (
	"context"
	"fmt"

	"github.com/glassflow/glassflow-cli/internal/config"
	"github.com/glassflow/glassflow-cli/internal/demo"
	"github.com/glassflow/glassflow-cli/internal/helm"
	"github.com/glassflow/glassflow-cli/internal/install"
	"github.com/glassflow/glassflow-cli/internal/k8s"
	"github.com/spf13/cobra"
)

var setupDemoCmd = &cobra.Command{
	Use:   "setup-demo",
	Short: "Install Kafka + ClickHouse and set up demo pipeline",
	Long:  `Install Kafka and ClickHouse into the Kind cluster, then set up the demo pipeline: create ClickHouse table, create GlassFlow pipeline, and start the Kafka producer. Run after 'glassflow up' has completed successfully.`,
	RunE:  runSetupDemo,
}

func init() {
	rootCmd.AddCommand(setupDemoCmd)
}

func runSetupDemo(cmd *cobra.Command, args []string) error {
	// Set version for demo package to use for GitHub downloads
	demo.SetVersion(version)

	fmt.Println("🎬 Setting up demo environment...")

	// Load configuration
	cfg, err := config.Load(configPath, version)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	kubeContext := resolveKubeContext(cfg)

	// Initialize managers
	k8sManager := k8s.NewManager(&k8s.Config{
		ClusterName: cfg.KindClusterName,
		Namespace:   "glassflow",
		Kubeconfig:  cfg.Kubeconfig,
		Context:     kubeContext,
	})

	// Check if cluster exists
	ctx := context.Background()
	status, err := k8sManager.GetClusterStatus(ctx)
	if err != nil {
		return fmt.Errorf("failed to check cluster status: %w", err)
	}

	if status.Status != "Running" {
		return fmt.Errorf("cluster '%s' is not running. Please run 'glassflow up' first", cfg.KindClusterName)
	}

	// Get Kubernetes client
	client, err := k8sManager.GetKubernetesClient()
	if err != nil {
		return fmt.Errorf("failed to get Kubernetes client: %w", err)
	}

	helmManager := helm.NewManager(client, &helm.Config{
		Namespace:    "glassflow",
		Kubeconfig:   cfg.Kubeconfig,
		Context:      kubeContext,
		Repositories: []helm.Repository{},
		Verbose:      verbose,
	})

	installManager := install.NewManager(helmManager, k8sManager, &install.Config{
		Namespace:   "glassflow",
		Demo:        true,
		Charts:      &cfg.Charts,
		KubeContext: kubeContext,
	})

	// Load demo image bundle if available
	loadImageBundle(cfg.KindClusterName, "demo-images")

	// Install Kafka and ClickHouse
	if err := installManager.InstallKafkaAndClickHouse(ctx); err != nil {
		return fmt.Errorf("failed to install Kafka/ClickHouse: %w", err)
	}

	// Wait for Kafka and ClickHouse to be ready
	fmt.Println("⏳ Waiting for Kafka and ClickHouse to be ready...")
	if err := installManager.WaitForKafkaAndClickHouseReady(ctx); err != nil {
		return fmt.Errorf("failed to wait for services: %w", err)
	}

	// Set up port forwarding
	fmt.Println("🔗 Setting up port forwarding...")
	portMapping, err := k8s.SetupPortForwarding(kubeContext, true)
	if err != nil {
		return fmt.Errorf("failed to setup port forwarding: %w", err)
	}

	// Setup demo pipeline
	if err := installManager.SetupDemo(ctx, portMapping); err != nil {
		return fmt.Errorf("failed to setup demo: %w", err)
	}

	fmt.Println("✅ Demo pipeline setup complete!")
	return nil
}
