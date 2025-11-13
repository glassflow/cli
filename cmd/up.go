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
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Initialize managers
	k8sManager := k8s.NewManager(&k8s.Config{
		ClusterName: cfg.KindClusterName,
		Namespace:   "glassflow",
	})

	ctx := context.Background()
	status, err := k8sManager.GetClusterStatus(ctx)
	if err != nil {
		return fmt.Errorf("failed to check cluster status: %w", err)
	}

	// Create Kind cluster if it doesn't exist
	if status.Status != "Running" {
		// Create Kind cluster
		if err := k8sManager.CreateCluster(ctx); err != nil {
			return fmt.Errorf("failed to create Kind cluster: %w", err)
		}
	} else {
		fmt.Printf("ℹ️  Cluster '%s' already exists, proceeding with service installation...\n", cfg.KindClusterName)
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
		Namespace:   "glassflow",
		Demo:        upOptions.Demo,
		Charts:      &cfg.Charts,
		KubeContext: cfg.Context,
	})

	// Check port availability before starting installation
	if upOptions.Demo {
		fmt.Println("🔍 Checking required ports availability...")
		requiredPorts := []struct {
			port int
			name string
		}{
			{30180, "GlassFlow API"},
			{30080, "GlassFlow UI"},
			{30090, "ClickHouse HTTP"},
		}

		var occupiedPorts []string
		for _, p := range requiredPorts {
			if !k8s.IsPortAvailable(p.port) {
				occupiedPorts = append(occupiedPorts, fmt.Sprintf("%s (port %d)", p.name, p.port))
			}
		}

		if len(occupiedPorts) > 0 {
			fmt.Printf("⚠️  Warning: The following ports are already in use:\n")
			for _, p := range occupiedPorts {
				fmt.Printf("   - %s\n", p)
			}
			fmt.Printf("💡 The CLI will attempt to find alternative ports, but services may fail to connect.\n")
			fmt.Printf("💡 To free up ports, stop other services using them or kill existing port-forwards:\n")
			fmt.Printf("   pkill -f 'kubectl port-forward'\n")
			fmt.Println()
		} else {
			fmt.Println("✅ All required ports are available")
		}
		fmt.Println()
	}

	// Start environment (installs services)
	if err := installManager.StartEnvironment(ctx, &install.StartOptions{
		IncludeDemo: upOptions.Demo,
		Namespace:   "glassflow",
	}); err != nil {
		return fmt.Errorf("failed to start environment: %w", err)
	}

	// Set up port forwarding after services are installed (needed for demo setup)
	if upOptions.Demo {
		// Wait for services to be ready before setting up port forwarding
		fmt.Println("⏳ Waiting for services to be ready before port forwarding...")
		if err := installManager.WaitForServicesReady(ctx); err != nil {
			return fmt.Errorf("failed to wait for services: %w", err)
		}

		fmt.Println("🔗 Setting up port forwarding...")
		portMapping, err := k8s.SetupPortForwarding(cfg.Context)
		if err != nil {
			return fmt.Errorf("failed to setup port forwarding: %w", err)
		}

		// Now setup demo pipeline (table creation and pipeline creation)
		if err := installManager.SetupDemo(ctx, portMapping); err != nil {
			return fmt.Errorf("failed to setup demo: %w", err)
		}
	}

	fmt.Println("✅ GlassFlow environment is ready!")

	return nil
}
