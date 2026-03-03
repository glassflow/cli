package cmd

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/glassflow/glassflow-cli/internal/config"
	"github.com/glassflow/glassflow-cli/internal/demo"
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
	Short: "Start local development environment (install only)",
	Long:  `Start a local GlassFlow development environment: create Kind cluster and install GlassFlow, Kafka, and ClickHouse. Waits until services are ready (can take 10–20+ minutes). Then run 'glassflow setup-demo' to create the demo pipeline. Use --demo to run install and demo in one go.`,
	RunE:  runUp,
}

func init() {
	rootCmd.AddCommand(upCmd)

	upCmd.Flags().BoolVar(&upOptions.Demo, "demo", false, "After install, also set up port-forwarding and demo pipeline (default: install only)")
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

	// Set version for demo package to use for GitHub downloads
	demo.SetVersion(version)

	fmt.Println("🚀 Starting GlassFlow local development environment...")

	// Preflight: verify a Docker-compatible runtime is available
	if err := checkDockerRuntime(); err != nil {
		return err
	}

	// Load configuration
	cfg, err := config.Load(configPath, version)
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

	// Start environment (always install GlassFlow + Kafka + ClickHouse so cluster is ready for setup-demo)
	if err := installManager.StartEnvironment(ctx, &install.StartOptions{
		IncludeDemo: true, // always install all charts; --demo only controls whether we run port-forward + pipeline
		Namespace:   "glassflow",
	}); err != nil {
		return fmt.Errorf("failed to start environment: %w", err)
	}

	// Always wait for services to be ready after install (so user knows when cluster is usable)
	fmt.Println("⏳ Waiting for services to be ready (this can take 10–20+ minutes on first run)...")
	if err := installManager.WaitForServicesReady(ctx); err != nil {
		return fmt.Errorf("failed to wait for services: %w", err)
	}

	fmt.Println("✅ GlassFlow environment is ready!")

	// Start port forwarding (runs in background and persists after CLI exits)
	fmt.Println("🔗 Setting up port forwarding...")
	portMapping, err := k8s.SetupPortForwarding(cfg.Context)
	if err != nil {
		return fmt.Errorf("failed to setup port forwarding: %w", err)
	}
	if portMapping != nil {
		fmt.Println("💡 Port forwarding is running in the background. Use 'glassflow down' to stop it.")
	}

	// If --demo, also run demo setup (table + pipeline + producer)
	if upOptions.Demo {
		if err := installManager.SetupDemo(ctx, portMapping); err != nil {
			return fmt.Errorf("failed to setup demo: %w", err)
		}
		fmt.Println("✅ Demo pipeline is ready!")
	} else {
		fmt.Println("💡 To set up the demo pipeline (ClickHouse table + pipeline + producer), run:")
		fmt.Println("   glassflow setup-demo")
	}

	return nil
}
