package cmd

import (
	"context"
	"fmt"

	"github.com/glassflow/glassflow-cli/internal/config"
	"github.com/glassflow/glassflow-cli/internal/helm"
	"github.com/glassflow/glassflow-cli/internal/install"
	"github.com/glassflow/glassflow-cli/internal/k8s"
	"github.com/spf13/cobra"
)

var setupDemoCmd = &cobra.Command{
	Use:   "setup-demo",
	Short: "Set up demo pipeline (table creation and pipeline creation)",
	Long:  `Set up the demo pipeline by creating ClickHouse table and GlassFlow pipeline. This requires services to be already installed and port-forwarding to be active.`,
	RunE:  runSetupDemo,
}

func init() {
	rootCmd.AddCommand(setupDemoCmd)
}

func runSetupDemo(cmd *cobra.Command, args []string) error {
	fmt.Println("🎬 Setting up demo pipeline...")

	// Load configuration
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Initialize managers
	k8sManager := k8s.NewManager(&k8s.Config{
		ClusterName: cfg.KindClusterName,
		Namespace:   "glassflow",
	})

	// Check if cluster exists
	ctx := context.Background()
	status, err := k8sManager.GetClusterStatus(ctx)
	if err != nil {
		return fmt.Errorf("failed to check cluster status: %w", err)
	}

	if status.Status != "Running" {
		return fmt.Errorf("cluster '%s' is not running. Please run 'glassflow up --demo' first", cfg.KindClusterName)
	}

	// Get Kubernetes client
	client, err := k8sManager.GetKubernetesClient()
	if err != nil {
		return fmt.Errorf("failed to get Kubernetes client: %w", err)
	}

	helmManager := helm.NewManager(client, &helm.Config{
		Namespace:    "glassflow",
		Kubeconfig:   cfg.Kubeconfig,
		Context:      cfg.Context,
		Repositories: []helm.Repository{},
	})

	installManager := install.NewManager(helmManager, k8sManager, &install.Config{
		Namespace:   "glassflow",
		Demo:        true,
		Charts:      &cfg.Charts,
		KubeContext: cfg.Context,
	})

	// Set up port forwarding
	fmt.Println("🔗 Setting up port forwarding...")
	portMapping, err := k8s.SetupPortForwarding(cfg.Context)
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
