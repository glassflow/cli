package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/glassflow/glassflow-cli/internal/config"
	"github.com/glassflow/glassflow-cli/internal/demo"
	"github.com/glassflow/glassflow-cli/internal/helm"
	"github.com/glassflow/glassflow-cli/internal/install"
	"github.com/glassflow/glassflow-cli/internal/k8s"
	"github.com/glassflow/glassflow-cli/internal/tracking"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

type UpOptions struct {
	Demo bool
}

var upOptions = &UpOptions{}

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Start local GlassFlow environment",
	Long:  `Start a local GlassFlow development environment: create Kind cluster and install GlassFlow. Use --demo to also install Kafka, ClickHouse, and set up a demo pipeline with data flowing end-to-end.`,
	RunE:  runUp,
}

func init() {
	rootCmd.AddCommand(upCmd)

	upCmd.Flags().BoolVar(&upOptions.Demo, "demo", false, "Also install Kafka + ClickHouse and set up a demo pipeline")
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

// loadImageBundle loads a tar.gz image bundle from ~/.glassflow/ into the Kind cluster.
// Returns true if images were loaded, false if bundle not found (non-fatal).
func loadImageBundle(clusterName, bundleName string) bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	gzPath := filepath.Join(home, ".glassflow", bundleName+".tar.gz")
	tarPath := filepath.Join(home, ".glassflow", bundleName+".tar")

	archiveToLoad := ""
	if _, err := os.Stat(gzPath); err == nil {
		fmt.Printf("📦 Decompressing %s...\n", bundleName)
		gunzip := exec.Command("gunzip", "-k", "-f", gzPath)
		if err := gunzip.Run(); err != nil {
			fmt.Printf("⚠️  Failed to decompress %s: %v\n", bundleName, err)
			return false
		}
		archiveToLoad = tarPath
	} else if _, err := os.Stat(tarPath); err == nil {
		archiveToLoad = tarPath
	} else {
		return false
	}

	fmt.Printf("📦 Loading %s into cluster...\n", bundleName)
	cmd := exec.Command("kind", "load", "image-archive", archiveToLoad, "--name", clusterName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Printf("⚠️  Failed to load %s: %v\n", bundleName, err)
		fmt.Println("   Continuing without pre-loaded images (pods will pull images normally).")
		return false
	}
	fmt.Printf("✅ %s loaded\n", bundleName)
	return true
}

func runUp(cmd *cobra.Command, args []string) (err error) {
	installationID := uuid.New().String()
	startTime := time.Now()
	defer func() {
		elapsed := time.Since(startTime)
		if err != nil {
			tracking.TrackUpFailed(installationID, version, upOptions.Demo, err, elapsed)
		} else {
			tracking.TrackUpCompleted(installationID, version, upOptions.Demo, elapsed)
		}
	}()

	if verbose {
		fmt.Printf("Starting GlassFlow environment with demo=%v, namespace=%s\n", upOptions.Demo, "glassflow")
	}

	// Set version for demo package to use for GitHub downloads
	demo.SetVersion(version)

	fmt.Println("🚀 Starting GlassFlow local development environment...")
	tracking.TrackUpStarted(installationID, version, upOptions.Demo)

	// Preflight: verify a Docker-compatible runtime is available
	if err = checkDockerRuntime(); err != nil {
		return err
	}

	// Load configuration
	cfg, err := config.Load(configPath, version)
	if err != nil {
		err = fmt.Errorf("failed to load config: %w", err)
		return err
	}
	kubeContext := resolveKubeContext(cfg)

	// Initialize managers
	k8sManager := k8s.NewManager(&k8s.Config{
		ClusterName: cfg.KindClusterName,
		Namespace:   "glassflow",
		Kubeconfig:  cfg.Kubeconfig,
		Context:     kubeContext,
	})

	ctx := context.Background()
	status, err := k8sManager.GetClusterStatus(ctx)
	if err != nil {
		err = fmt.Errorf("failed to check cluster status: %w", err)
		return err
	}

	// Create Kind cluster if it doesn't exist
	if status.Status != "Running" {
		// Create Kind cluster
		if err = k8sManager.CreateCluster(ctx); err != nil {
			err = fmt.Errorf("failed to create Kind cluster: %w", err)
			return err
		}
	} else {
		fmt.Printf("ℹ️  Cluster '%s' already exists, proceeding with service installation...\n", cfg.KindClusterName)
	}

	// Wait for cluster to be ready (API + nodes Ready)
	if err = k8sManager.WaitForClusterReady(ctx, 1*time.Minute); err != nil {
		return err
	}

	// Load pre-built image bundles if available (speeds up first install significantly)
	loadImageBundle(cfg.KindClusterName, "glassflow-images")
	if upOptions.Demo {
		loadImageBundle(cfg.KindClusterName, "demo-images")
	}

	// Now get the Kubernetes client
	client, err := k8sManager.GetKubernetesClient()
	if err != nil {
		err = fmt.Errorf("failed to get Kubernetes client: %w", err)
		return err
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
		Demo:        upOptions.Demo,
		Charts:      &cfg.Charts,
		KubeContext: kubeContext,
	})

	// Start environment
	if err = installManager.StartEnvironment(ctx, &install.StartOptions{
		IncludeDemo: upOptions.Demo,
		Namespace:   "glassflow",
	}); err != nil {
		err = fmt.Errorf("failed to start environment: %w", err)
		return err
	}

	// Wait for GlassFlow services (and Kafka/ClickHouse if --demo)
	fmt.Println("⏳ Waiting for services to be ready...")
	if err = installManager.WaitForServicesReady(ctx); err != nil {
		err = fmt.Errorf("failed to wait for services: %w", err)
		return err
	}

	fmt.Println("✅ GlassFlow environment is ready!")

	// Start port forwarding (runs in background and persists after CLI exits)
	fmt.Println("🔗 Setting up port forwarding...")
	portMapping, err := k8s.SetupPortForwarding(kubeContext, upOptions.Demo)
	if err != nil {
		err = fmt.Errorf("failed to setup port forwarding: %w", err)
		return err
	}
	if portMapping != nil {
		fmt.Println("💡 Port forwarding is running in the background. Use 'glassflow down' to stop it.")
	}

	// If --demo, also run demo setup (table + pipeline + producer)
	if upOptions.Demo {
		if err = installManager.SetupDemo(ctx, portMapping); err != nil {
			err = fmt.Errorf("failed to setup demo: %w", err)
			return err
		}
		fmt.Println("✅ Demo pipeline is ready!")
	} else {
		fmt.Println("💡 To set up the demo pipeline (Kafka + ClickHouse + demo pipeline), run:")
		fmt.Println("   glassflow setup-demo")
	}

	return nil
}
