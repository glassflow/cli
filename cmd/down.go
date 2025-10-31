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

type DownOptions struct {
	Force bool
}

var downOptions = &DownOptions{}

var downCmd = &cobra.Command{
	Use:   "down",
	Short: "Stop local development environment",
	Long:  `Stop the local GlassFlow development environment and clean up resources.`,
	RunE:  runDown,
}

func init() {
	rootCmd.AddCommand(downCmd)

	downCmd.Flags().BoolVar(&downOptions.Force, "force", false, "Force cleanup even if resources are in use")
}

func runDown(cmd *cobra.Command, args []string) error {
	if verbose {
		fmt.Printf("Stopping GlassFlow environment in namespace=glassflow, force=%v\n", downOptions.Force)
	}

	fmt.Println("🛑 Stopping GlassFlow local development environment...")

	// Load configuration
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Clean up only the port-forwards started by our CLI
	fmt.Println("🔗 Cleaning up port forwarding...")
	k8s.CleanupPortForwarding(verbose)

	// Override namespace if specified
	if cfg.Namespace != "glassflow" {
		cfg.Namespace = "glassflow"
	}

	// Initialize managers
	k8sManager := k8s.NewManager(&k8s.Config{
		ClusterName: cfg.KindClusterName,
		Namespace:   cfg.Namespace,
	})

	client, err := k8sManager.GetKubernetesClient()
	if err != nil {
		return fmt.Errorf("failed to get Kubernetes client: %w", err)
	}

	helmManager := helm.NewManager(client, &helm.Config{
		Namespace:    cfg.Namespace,
		Kubeconfig:   cfg.Kubeconfig,
		Context:      cfg.Context,
		Repositories: []helm.Repository{},
	})

	installManager := install.NewManager(helmManager, k8sManager, &install.Config{
		Namespace: cfg.Namespace,
		Demo:      true, // Assume demo was enabled TODO - remove this once we have a way to determine if demo was enabled
		Charts:    &cfg.Charts,
	})

	// Stop environment
	ctx := context.Background()
	if downOptions.Force {
		// Force mode: directly delete the Kind cluster without waiting for Helm uninstallation
		fmt.Println("⚠️  Force mode: directly deleting Kind cluster...")
		if err := k8sManager.DeleteCluster(ctx); err != nil {
			return fmt.Errorf("failed to delete Kind cluster: %w", err)
		}
	} else {
		// Normal mode: try to uninstall Helm releases first, then delete cluster
		if err := installManager.StopEnvironment(ctx); err != nil {
			return fmt.Errorf("failed to stop environment: %w", err)
		}
	}

	fmt.Println("✅ GlassFlow environment stopped successfully!")

	return nil
}
