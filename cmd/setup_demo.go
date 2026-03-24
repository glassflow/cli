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
	Short: "Set up demo pipeline (port-forward, table, pipeline, producer)",
	Long:  `Set up the demo pipeline: start port-forwarding, create ClickHouse table, create GlassFlow pipeline, and start the Kafka producer. Run after 'glassflow up' has completed successfully.`,
	RunE:  runSetupDemo,
}

func init() {
	rootCmd.AddCommand(setupDemoCmd)
}

func runSetupDemo(cmd *cobra.Command, args []string) error {
	// Set version for demo package to use for GitHub downloads
	demo.SetVersion(version)

	fmt.Println("🎬 Setting up demo pipeline...")

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
		return fmt.Errorf("cluster '%s' is not running. Please run 'glassflow up' first, then run this command", cfg.KindClusterName)
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

	// Set up port forwarding
	fmt.Println("🔗 Setting up port forwarding...")
	portMapping, err := k8s.SetupPortForwarding(kubeContext)
	if err != nil {
		return fmt.Errorf("failed to setup port forwarding: %w", err)
	}
	// Port forwards are started in background, no need to wait

	// Setup demo pipeline
	if err := installManager.SetupDemo(ctx, portMapping); err != nil {
		return fmt.Errorf("failed to setup demo: %w", err)
	}

	fmt.Println("✅ Demo pipeline setup complete!")
	return nil
}
