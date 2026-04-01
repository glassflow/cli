package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/glassflow/glassflow-cli/internal/config"
	"github.com/glassflow/glassflow-cli/internal/demo"
	"github.com/glassflow/glassflow-cli/internal/helm"
	"github.com/glassflow/glassflow-cli/internal/install"
	"github.com/glassflow/glassflow-cli/internal/k8s"
	"github.com/glassflow/glassflow-cli/internal/tracking"
	"github.com/spf13/cobra"
)

type UpOptions struct {
	Demo           bool
	SkipPreflight  bool
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
	upCmd.Flags().BoolVar(&upOptions.SkipPreflight, "skip-preflight", false, "Skip preflight checks (Docker resources, binary checks)")
}

// runPreflightChecks validates all prerequisites before starting the environment.
// Collects all errors and reports them together so the user can fix everything at once.
func runPreflightChecks() error {
	fmt.Println("🔍 Running preflight checks...")
	var errors []string
	var warnings []string

	// Check Docker
	dockerCmd := exec.Command("docker", "info", "--format", "json")
	dockerOut, err := dockerCmd.Output()
	if err != nil {
		errors = append(errors, "Docker is not running. Please install and start a Docker-compatible runtime (Docker Desktop, OrbStack, Colima, or Podman).")
	} else {
		// Parse Docker info for resource checks
		var info struct {
			MemTotal int64 `json:"MemTotal"`
			NCPU     int   `json:"NCPU"`
		}
		if json.Unmarshal(dockerOut, &info) == nil {
			memGB := float64(info.MemTotal) / (1024 * 1024 * 1024)
			if memGB < 2 {
				errors = append(errors, fmt.Sprintf("Docker has %.1f GB RAM. GlassFlow requires at least 4 GB. Update in Docker Desktop > Settings > Resources.", memGB))
			} else if memGB < 4 {
				warnings = append(warnings, fmt.Sprintf("Docker has %.1f GB RAM. GlassFlow recommends at least 4 GB for reliable operation. Update in Docker Desktop > Settings > Resources.", memGB))
			}
			if info.NCPU < 2 {
				warnings = append(warnings, fmt.Sprintf("Docker has %d CPU(s). GlassFlow recommends at least 2 CPUs.", info.NCPU))
			}
		}
	}

	// Check Helm
	if _, err := exec.LookPath("helm"); err != nil {
		errors = append(errors, "helm is not installed. Install with: brew install helm (or see https://helm.sh/docs/intro/install/)")
	}

	// Check kubectl
	if _, err := exec.LookPath("kubectl"); err != nil {
		errors = append(errors, "kubectl is not installed. Install with: brew install kubectl (or see https://kubernetes.io/docs/tasks/tools/)")
	}

	// Report warnings
	for _, w := range warnings {
		fmt.Printf("   ⚠️  %s\n", w)
	}

	// Report errors
	if len(errors) > 0 {
		fmt.Println()
		for _, e := range errors {
			fmt.Printf("   ❌ %s\n", e)
		}
		fmt.Println()
		return fmt.Errorf("preflight checks failed (%d error(s)). Fix the issues above and try again", len(errors))
	}

	fmt.Println("   ✅ All preflight checks passed")
	return nil
}

// downloadFile downloads a URL to a local file path. Returns nil on success.
func downloadFile(url, destPath string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}
	tmp := destPath + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	f.Close()
	return os.Rename(tmp, destPath)
}

// invalidateStaleCache removes cached image bundles if the CLI version has changed.
func invalidateStaleCache() {
	if version == "" || version == "dev" {
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	dir := filepath.Join(home, ".glassflow")
	versionFile := filepath.Join(dir, "images_version")

	data, err := os.ReadFile(versionFile)
	if err == nil && strings.TrimSpace(string(data)) == version {
		return // version matches, cache is valid
	}

	// Version mismatch or no version file — remove stale bundles
	for _, name := range []string{"glassflow-images.tar.gz", "glassflow-images.tar", "demo-images.tar.gz", "demo-images.tar"} {
		os.Remove(filepath.Join(dir, name))
	}

	// Write current version
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(versionFile, []byte(version), 0o644)
}

// ensureImageBundle checks for the image bundle locally, downloads from GitHub release if missing.
func ensureImageBundle(bundleName string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	gzPath := filepath.Join(home, ".glassflow", bundleName+".tar.gz")

	// Already cached locally
	if _, err := os.Stat(gzPath); err == nil {
		return gzPath
	}

	// Skip download for dev builds
	if version == "" || version == "dev" {
		return ""
	}

	url := fmt.Sprintf("https://github.com/glassflow/cli/releases/download/v%s/%s.tar.gz", version, bundleName)
	fmt.Printf("📥 Downloading %s (first run only)...\n", bundleName)
	if err := downloadFile(url, gzPath); err != nil {
		fmt.Printf("⚠️  Download failed: %v\n", err)
		fmt.Println("   Pods will pull images normally (slower first run).")
		return ""
	}
	fmt.Printf("✅ Downloaded %s\n", bundleName)
	return gzPath
}

// loadImageBundle downloads (if needed) and loads an image bundle into the Kind cluster.
// Returns true if images were loaded, false otherwise (non-fatal).
func loadImageBundle(clusterName, bundleName string) bool {
	gzPath := ensureImageBundle(bundleName)
	if gzPath == "" {
		return false
	}

	tarPath := gzPath[:len(gzPath)-3] // strip .gz
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
	startTime := time.Now()
	defer func() {
		elapsed := time.Since(startTime)
		if err != nil {
			tracking.TrackUpFailed(version, upOptions.Demo, err, elapsed)
		} else {
			tracking.TrackUpCompleted(version, upOptions.Demo, elapsed)
		}
	}()

	if verbose {
		fmt.Printf("Starting GlassFlow environment with demo=%v, namespace=%s\n", upOptions.Demo, "glassflow")
	}

	// Set version for demo package to use for GitHub downloads
	demo.SetVersion(version)

	fmt.Println("🚀 Starting GlassFlow local development environment...")
	tracking.TrackUpStarted(version, upOptions.Demo)

	// Preflight checks
	if !upOptions.SkipPreflight {
		if err = runPreflightChecks(); err != nil {
			return err
		}
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

	// Invalidate cached image bundles if CLI version changed
	invalidateStaleCache()

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
		Namespace:      "glassflow",
		Demo:           upOptions.Demo,
		Charts:         &cfg.Charts,
		KubeContext:    kubeContext,
		InstallationID: tracking.GetInstallationID(),
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
