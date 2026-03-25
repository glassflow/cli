package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/glassflow/glassflow-cli/internal/config"
	"github.com/glassflow/glassflow-cli/internal/k8s"
	"github.com/glassflow/glassflow-cli/internal/tracking"
	"github.com/spf13/cobra"
)

var downCmd = &cobra.Command{
	Use:   "down",
	Short: "Stop local development environment",
	Long:  `Stop the local GlassFlow development environment: kill port forwards and delete the Kind cluster.`,
	RunE:  runDown,
}

var forceDown bool

func init() {
	rootCmd.AddCommand(downCmd)
	// Kept for backward compatibility — glassflow down always deletes the cluster directly
	downCmd.Flags().BoolVar(&forceDown, "force", false, "Kept for backward compatibility (no-op, cluster is always deleted directly)")
	_ = downCmd.Flags().MarkHidden("force")
}

func runDown(cmd *cobra.Command, args []string) (err error) {
	startTime := time.Now()
	defer func() {
		elapsed := time.Since(startTime)
		if err != nil {
			tracking.TrackDownFailed(version, false, err, elapsed)
		} else {
			tracking.TrackDownCompleted(version, false, elapsed)
		}
	}()

	fmt.Println("🛑 Stopping GlassFlow local development environment...")

	// Load configuration
	cfg, err := config.Load(configPath, version)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Clean up port-forwards started by our CLI
	fmt.Println("🔗 Cleaning up port forwarding...")
	k8s.CleanupPortForwarding(verbose)

	// Delete the Kind cluster (removes all Helm releases, pods, PVCs with it)
	k8sManager := k8s.NewManager(&k8s.Config{
		ClusterName: cfg.KindClusterName,
		Namespace:   "glassflow",
		Kubeconfig:  cfg.Kubeconfig,
		Context:     resolveKubeContext(cfg),
	})

	ctx := context.Background()
	if err := k8sManager.DeleteCluster(ctx); err != nil {
		return fmt.Errorf("failed to delete Kind cluster: %w", err)
	}

	fmt.Println("✅ GlassFlow environment stopped successfully!")
	return nil
}
