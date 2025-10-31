package cmd

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/glassflow/glassflow-cli/internal/config"
	"github.com/glassflow/glassflow-cli/internal/helm"
	"github.com/glassflow/glassflow-cli/internal/install"
	"github.com/glassflow/glassflow-cli/internal/k8s"
	"github.com/spf13/cobra"
)

type UpOptions struct {
	Demo bool
}

var upOptions = &UpOptions{}

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Start local development environment",
	Long:  `Start a local GlassFlow development environment with Kind cluster, Kafka, ClickHouse, and demo data.`,
	RunE:  runUp,
}

func init() {
	rootCmd.AddCommand(upCmd)

	upCmd.Flags().BoolVar(&upOptions.Demo, "demo", true, "Include demo data and pipelines (default: true)")
}

// checkDockerRuntime ensures a Docker-compatible runtime is available by invoking `docker info`.
// We intentionally do not detect specific providers; users can choose any Docker-compatible runtime.
func checkDockerRuntime() error {
	cmd := exec.Command("docker", "info")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("no Docker-compatible runtime detected. Please install and start a Docker-compatible runtime (e.g., Docker Desktop, OrbStack, Colima, or Podman) and ensure 'docker info' succeeds: %w", err)
	}
	return nil
}

func runUp(cmd *cobra.Command, args []string) error {
	if verbose {
		fmt.Printf("Starting GlassFlow environment with demo=%v, namespace=%s\n", upOptions.Demo, "glassflow")
	}

	fmt.Println("🚀 Starting GlassFlow local development environment...")

	// Preflight: verify a Docker-compatible runtime is available
	if err := checkDockerRuntime(); err != nil {
		return err
	}

	// Load configuration
	cfg, err := config.Load("")
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Initialize managers
	k8sManager := k8s.NewManager(&k8s.Config{
		ClusterName: cfg.KindClusterName,
		Namespace:   "glassflow",
	})

	// Always ensure clean cluster state to avoid broken installations
	ctx := context.Background()
	status, err := k8sManager.GetClusterStatus(ctx)
	if err != nil {
		return fmt.Errorf("failed to check cluster status: %w", err)
	}

	if status.Status == "Running" {
		return fmt.Errorf("kind cluster '%s' already exists. please run 'glassflow down' to stop and clean up before running 'glassflow up' again", cfg.KindClusterName)
	}

	// Create Kind cluster
	if err := k8sManager.CreateCluster(ctx); err != nil {
		return fmt.Errorf("failed to create Kind cluster: %w", err)
	}

	// Wait for cluster to be ready (API + nodes Ready)
	if err := k8sManager.WaitForClusterReady(ctx, 1*time.Minute); err != nil {
		return err
	}

	// Now get the Kubernetes client
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
		Namespace: "glassflow",
		Demo:      upOptions.Demo,
		Charts:    &cfg.Charts,
	})

	// Start environment
	if err := installManager.StartEnvironment(ctx, &install.StartOptions{
		IncludeDemo: upOptions.Demo,
		Namespace:   "glassflow",
	}); err != nil {
		return fmt.Errorf("failed to start environment: %w", err)
	}

	// Set up port forwarding for demo mode
	if upOptions.Demo {
		fmt.Println("🔗 Setting up port forwarding...")
		if err := k8s.SetupPortForwarding(cfg.Context); err != nil {
			fmt.Printf("⚠️  Warning: Port-forward setup issue: %v\n", err)
		}
	}

	fmt.Println("✅ GlassFlow environment is ready!")

	return nil
}
