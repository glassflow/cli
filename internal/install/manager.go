package install

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/glassflow/glassflow-cli/internal/config"
	"github.com/glassflow/glassflow-cli/internal/demo"
	"github.com/glassflow/glassflow-cli/internal/helm"
	"github.com/glassflow/glassflow-cli/internal/k8s"
)

//go:embed glassflow_values.yaml
var glassflowValuesYAML []byte

// Deterministic credentials and namespaces for demo setup
const (
	// Kafka credentials
	DemoKafkaUsername = "user1"
	DemoKafkaPassword = "glassflow-demo-password"

	// ClickHouse credentials
	DemoClickHouseUsername = "default"
	DemoClickHousePassword = "glassflow-demo-password"

	// Namespaces for Kafka and ClickHouse (GlassFlow stays in config.Namespace, typically "glassflow")
	KafkaNamespace      = "kafka"
	ClickHouseNamespace = "clickhouse"

	// Wait timeouts for install phase (first run can take 10–20+ min if pods are scheduling)
	WaitForServicesTimeout        = 25 * time.Minute
	WaitForKafkaClickHouseTimeout = 20 * time.Minute
	WaitProgressInterval          = 30 * time.Second // print status and hints at this interval
)

type Manager struct {
	helmManager *helm.Manager
	k8sManager  *k8s.Manager
	config      *Config
}

type Config struct {
	Namespace   string
	Demo        bool
	Charts      *config.ChartsConfig
	KubeContext string // For port forwarding restarts
}

type StartOptions struct {
	IncludeDemo bool
	Namespace   string
}

func NewManager(helmManager *helm.Manager, k8sManager *k8s.Manager, config *Config) *Manager {
	return &Manager{
		helmManager: helmManager,
		k8sManager:  k8sManager,
		config:      config,
	}
}

func (i *Manager) StartEnvironment(ctx context.Context, opts *StartOptions) error {
	// Add Helm repositories
	if err := i.addRepositories(ctx); err != nil {
		return fmt.Errorf("failed to add repositories: %w", err)
	}

	if opts.IncludeDemo {
		// Install all charts in parallel for demo mode
		var wg sync.WaitGroup
		var glassflowErr, kafkaErr, clickhouseErr error

		wg.Add(3)

		// Install GlassFlow chart (no wait, runs in parallel)
		go func() {
			defer wg.Done()
			glassflowErr = i.installGlassFlow(ctx)
		}()

		// Install Kafka chart (no wait, runs in parallel)
		go func() {
			defer wg.Done()
			kafkaErr = i.installKafka(ctx)
		}()

		// Install ClickHouse chart (no wait, runs in parallel)
		go func() {
			defer wg.Done()
			clickhouseErr = i.installClickHouse(ctx)
		}()

		wg.Wait()

		// Check for errors
		if glassflowErr != nil {
			return fmt.Errorf("failed to install GlassFlow: %w", glassflowErr)
		}
		if kafkaErr != nil {
			return fmt.Errorf("failed to install Kafka: %w", kafkaErr)
		}
		if clickhouseErr != nil {
			return fmt.Errorf("failed to install ClickHouse: %w", clickhouseErr)
		}
	} else {
		// Non-demo mode: install GlassFlow only
		if err := i.installGlassFlow(ctx); err != nil {
			return fmt.Errorf("failed to install GlassFlow: %w", err)
		}
	}

	return nil
}

// SetupDemo sets up the demo pipeline (creates ClickHouse table and GlassFlow pipeline)
func (i *Manager) SetupDemo(ctx context.Context, portMapping *k8s.PortMapping) error {
	return i.setupDemo(ctx, portMapping)
}

func (i *Manager) StopEnvironment(ctx context.Context) error {
	// Uninstall GlassFlow chart
	if err := i.helmManager.UninstallChart(ctx, &helm.UninstallOptions{
		ReleaseName: "glassflow",
		Namespace:   i.config.Namespace,
		Wait:        true,
	}); err != nil {
		// Log error but continue with cleanup
		fmt.Printf("Warning: failed to uninstall GlassFlow: %v\n", err)
	}

	// Uninstall demo services (ignore if not found, we have force delete cluster option)
	if i.config.Demo {
		if err := i.helmManager.UninstallChart(ctx, &helm.UninstallOptions{
			ReleaseName: "kafka",
			Namespace:   KafkaNamespace,
			Wait:        true,
		}); err != nil {
			fmt.Printf("Warning: failed to uninstall Kafka: %v\n", err)
		}

		if err := i.helmManager.UninstallChart(ctx, &helm.UninstallOptions{
			ReleaseName: "clickhouse",
			Namespace:   ClickHouseNamespace,
			Wait:        true,
		}); err != nil {
			fmt.Printf("Warning: failed to uninstall ClickHouse: %v\n", err)
		}
	}

	// Delete Kind cluster
	if err := i.k8sManager.DeleteCluster(ctx); err != nil {
		return fmt.Errorf("failed to delete Kind cluster: %w", err)
	}

	return nil
}

func (i *Manager) addRepositories(ctx context.Context) error {
	repositories := []helm.Repository{
		{Name: "glassflow", URL: i.config.Charts.GlassFlow.Repository},
		{Name: "bitnami", URL: i.config.Charts.Kafka.Repository}, // TODO - use stable repos instead of bitnamilegacy
	}

	for _, repo := range repositories {
		if err := i.helmManager.AddRepository(ctx, &repo); err != nil {
			return fmt.Errorf("failed to add repository %s: %w", repo.Name, err)
		}
	}

	return nil
}

func (i *Manager) installGlassFlow(ctx context.Context) error {
	// Write embedded glassflow_values.yaml to a temp file so helm can use -f
	valuesFile, err := os.CreateTemp("", "glassflow_values_*.yaml")
	if err != nil {
		return fmt.Errorf("failed to create values file: %w", err)
	}
	valuesPath := valuesFile.Name()
	defer os.Remove(valuesPath)
	if _, err := valuesFile.Write(glassflowValuesYAML); err != nil {
		valuesFile.Close()
		return fmt.Errorf("failed to write values file: %w", err)
	}
	if err := valuesFile.Close(); err != nil {
		return fmt.Errorf("failed to close values file: %w", err)
	}

	_, err = i.helmManager.InstallChart(ctx, &helm.InstallOptions{
		Chart:           i.config.Charts.GlassFlow.Chart,
		Version:         i.config.Charts.GlassFlow.Version, // empty = use latest
		ReleaseName:     "glassflow",
		Namespace:       i.config.Namespace,
		ValuesFile:      valuesPath,
		CreateNamespace: true,
		Wait:            false, // Don't wait - allow parallel installation
		Timeout:         600,
	})
	return err
}

func (i *Manager) installKafka(ctx context.Context) error {
	// Kafka 4.0+ uses KRaft mode (no Zookeeper)
	// According to Bitnami Kafka chart docs, we need to create the secret first
	// and use existingSecret parameter, as the chart generates random passwords
	// when using auth.sasl.client.passwords directly
	k8sClient, err := i.k8sManager.GetKubernetesClient()
	if err != nil {
		return fmt.Errorf("failed to get k8s client: %w", err)
	}

	secretName := "kafka-user-passwords"
	releaseName := "kafka"

	// Ensure Kafka namespace exists before creating secret
	_, err = k8sClient.CoreV1().Namespaces().Get(ctx, KafkaNamespace, metav1.GetOptions{})
	if err != nil {
		// Check if error is "not found" - if so, create the namespace
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "NotFound") {
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: KafkaNamespace,
				},
			}
			if _, createNsErr := k8sClient.CoreV1().Namespaces().Create(ctx, namespace, metav1.CreateOptions{}); createNsErr != nil {
				// Ignore error if namespace already exists (race condition with parallel installs)
				if !strings.Contains(createNsErr.Error(), "already exists") {
					fmt.Printf("⚠️  Warning: Failed to create namespace %s: %v\n", KafkaNamespace, createNsErr)
				}
			}
		} else {
			// Other error getting namespace - log but continue (Helm will create it)
			fmt.Printf("⚠️  Warning: Failed to check namespace %s: %v\n", KafkaNamespace, err)
		}
	}

	// Delete existing secret if it exists
	_ = k8sClient.CoreV1().Secrets(KafkaNamespace).Delete(ctx, secretName, metav1.DeleteOptions{})

	// Create secret with deterministic password and Helm labels for ownership
	// Note: Chart version in label is not critical - Helm will manage it
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: KafkaNamespace,
			Labels: map[string]string{
				"app.kubernetes.io/instance":   releaseName,
				"app.kubernetes.io/managed-by": "Helm",
				"app.kubernetes.io/name":       "kafka",
				// Chart version will be managed by Helm, no need to hardcode
			},
			Annotations: map[string]string{
				"meta.helm.sh/release-name":      releaseName,
				"meta.helm.sh/release-namespace": KafkaNamespace,
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"client-passwords":      []byte(DemoKafkaPassword),
			"inter-broker-password": []byte(DemoKafkaPassword),
			"controller-password":   []byte(DemoKafkaPassword),
			"system-user-password":  []byte(DemoKafkaPassword),
		},
	}

	// Create secret before Helm install so it can be used
	if _, createErr := k8sClient.CoreV1().Secrets(KafkaNamespace).Create(ctx, secret, metav1.CreateOptions{}); createErr != nil {
		fmt.Printf("⚠️  Warning: Failed to create Kafka secret: %v\n", createErr)
	} else {
		fmt.Printf("✅ Created Kafka secret with deterministic password\n")
	}

	// Kafka installation: 1 controller for local Kind dev (reduces storage: 25Gi + 1Gi logs).
	// KRaft supports single-node for dev; use 3 replicas in production.
	// Single broker requires offsets.topic.replication.factor=1 so __consumer_offsets can be created (avoids COORDINATOR_NOT_AVAILABLE).
	values := map[string]interface{}{
		"replicaCount": 1,
		"controller": map[string]interface{}{
			"replicaCount": 1,
			"persistence": map[string]interface{}{
				"enabled":      true,
				"size":         "25Gi",
				"storageClass": "",
			},
			"logPersistence": map[string]interface{}{
				"enabled":      true,
				"size":         "1Gi",
				"storageClass": "",
			},
		},
		// Required for single-broker: allow __consumer_offsets and transaction log to be created with 1 replica (avoids COORDINATOR_NOT_AVAILABLE)
		"overrideConfiguration": map[string]interface{}{
			"offsets.topic.replication.factor":            "1",
			"transaction.state.log.replication.factor":     "1",
		},
		"image": map[string]interface{}{
			"registry":   i.config.Charts.Kafka.Image.Registry,
			"repository": i.config.Charts.Kafka.Image.Repository,
		},
		"auth": map[string]interface{}{
			"clientProtocol": "sasl",
			"sasl": map[string]interface{}{
				"enabled":                   true,
				"mechanisms":                []string{"plain"},
				"interBrokerMechanism":      "plain",
				"allowEveryoneIfNoAclFound": true,
				"existingSecret":            secretName, // Use our pre-created secret
			},
		},
		// Note: Bitnami chart auto-generates controller quorum voters based on controller.replicaCount
		// No need to explicitly set KAFKA_CFG_CONTROLLER_QUORUM_VOTERS
	}

	_, err = i.helmManager.InstallChart(ctx, &helm.InstallOptions{
		Chart:           i.config.Charts.Kafka.Chart,
		Version:         i.config.Charts.Kafka.Version,
		ReleaseName:     "kafka",
		Namespace:       KafkaNamespace,
		Values:          values,
		CreateNamespace: true,
		Wait:            false, // Don't wait - allow parallel installation
		Timeout:         300,
	})

	return err
}

func (i *Manager) installClickHouse(ctx context.Context) error {
	// Use config values for images and disable persistence for local dev
	// Set deterministic password and single replica/shard for demo
	values := map[string]interface{}{
		"replicaCount": 1, // Single replica per shard
		"shards":       1, // Single shard for local dev
		"persistence": map[string]interface{}{
			"enabled": false, // Use emptyDir for local dev
		},
		"image": map[string]interface{}{
			"registry":   i.config.Charts.ClickHouse.Image.Registry,
			"repository": i.config.Charts.ClickHouse.Image.Repository,
		},
		"keeper": map[string]interface{}{
			"replicaCount": 1, // Single keeper for local dev
			"image": map[string]interface{}{
				"registry":   i.config.Charts.ClickHouse.Keeper.Registry,
				"repository": i.config.Charts.ClickHouse.Keeper.Repository,
			},
			"persistence": map[string]interface{}{
				"enabled": false,
			},
			// Reduce keeper resources
			"resources": map[string]interface{}{
				"requests": map[string]interface{}{
					"cpu":    "100m",
					"memory": "256Mi",
				},
				"limits": map[string]interface{}{
					"cpu":    "500m",
					"memory": "512Mi",
				},
			},
		},
		"auth": map[string]interface{}{
			"username": DemoClickHouseUsername,
			"password": DemoClickHousePassword,
		},
		// Reduce ClickHouse server resources
		"resources": map[string]interface{}{
			"requests": map[string]interface{}{
				"cpu":    "200m",
				"memory": "512Mi",
			},
			"limits": map[string]interface{}{
				"cpu":    "1000m",
				"memory": "2Gi",
			},
		},
	}

	_, err := i.helmManager.InstallChart(ctx, &helm.InstallOptions{
		Chart:           i.config.Charts.ClickHouse.Chart,
		Version:         i.config.Charts.ClickHouse.Version,
		ReleaseName:     "clickhouse",
		Namespace:       ClickHouseNamespace,
		Values:          values,
		CreateNamespace: true,
		Wait:            false, // Don't wait - allow parallel installation
		Timeout:         300,
	})

	return err
}

// WaitForServicesReady waits for GlassFlow services to be ready (have endpoints)
func (i *Manager) WaitForServicesReady(ctx context.Context) error {
	k8sClient, err := i.k8sManager.GetKubernetesClient()
	if err != nil {
		return fmt.Errorf("failed to get k8s client: %w", err)
	}

	// (namespace, serviceName) - GlassFlow in config namespace, ClickHouse in its own namespace
	services := []struct{ namespace, name, label string }{
		{i.config.Namespace, "glassflow-api", "glassflow-api"},
		{i.config.Namespace, "glassflow-ui", "glassflow-ui"},
		{ClickHouseNamespace, "clickhouse", "clickhouse"},
	}
	timeout := WaitForServicesTimeout
	deadline := time.Now().Add(timeout)
	startTime := time.Now()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	lastProgress := time.Now()
	fmt.Println("💡 If pods stay Pending, increase Docker memory/CPU (e.g. Docker Desktop → Settings → Resources).")

	for {
		if time.Now().After(deadline) {
			// Report which services are still not ready
			var notReady []string
			for _, svc := range services {
				endpoints, err := k8sClient.CoreV1().Endpoints(svc.namespace).Get(ctx, svc.name, metav1.GetOptions{})
				if err != nil {
					notReady = append(notReady, fmt.Sprintf("%s (ns: %s)", svc.name, svc.namespace))
					continue
				}
				hasAddr := false
				for _, subset := range endpoints.Subsets {
					if len(subset.Addresses) > 0 {
						hasAddr = true
						break
					}
				}
				if !hasAddr {
					notReady = append(notReady, fmt.Sprintf("%s (ns: %s)", svc.name, svc.namespace))
				}
			}
			return fmt.Errorf("timeout waiting for services after %v (not ready: %v). Check: kubectl get pods -n glassflow -n kafka -n clickhouse; kubectl describe nodes", timeout, notReady)
		}

		allReady := true
		var notReady []string
		for _, svc := range services {
			endpoints, err := k8sClient.CoreV1().Endpoints(svc.namespace).Get(ctx, svc.name, metav1.GetOptions{})
			if err != nil {
				allReady = false
				notReady = append(notReady, svc.label)
				continue
			}
			hasReadyEndpoint := false
			for _, subset := range endpoints.Subsets {
				if len(subset.Addresses) > 0 {
					hasReadyEndpoint = true
					break
				}
			}
			if !hasReadyEndpoint {
				allReady = false
				notReady = append(notReady, svc.label)
			}
		}

		if allReady {
			fmt.Println("✅ Services are ready")
			return nil
		}

		// Progress and debug hints at interval
		if time.Since(lastProgress) >= WaitProgressInterval {
			lastProgress = time.Now()
			elapsed := time.Since(startTime).Round(time.Second)
			fmt.Printf("⏳ Still waiting for: %v (%s elapsed). Check: kubectl get pods -n glassflow -n kafka -n clickhouse\n", notReady, elapsed)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			// Continue polling
		}
	}
}

// waitForKafkaAndClickHouse polls until both Kafka and ClickHouse pods are ready
func (i *Manager) waitForKafkaAndClickHouse(ctx context.Context) error {
	k8sClient, err := i.k8sManager.GetKubernetesClient()
	if err != nil {
		return fmt.Errorf("failed to get k8s client: %w", err)
	}

	timeout := WaitForKafkaClickHouseTimeout
	deadline := time.Now().Add(timeout)
	startTime := time.Now()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	lastProgress := time.Now()
	kafkaReadyCount := 0
	clickhouseReadyCount := 0

	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for Kafka and ClickHouse after %v. Check: kubectl get pods -n kafka -n clickhouse; kubectl describe nodes", timeout)
		}

		// Check Kafka readiness (in kafka namespace)
		kafkaReady := i.checkPodsReady(ctx, k8sClient, KafkaNamespace, "app.kubernetes.io/name=kafka")
		// Check ClickHouse readiness (in clickhouse namespace)
		clickhouseReady := i.checkPodsReady(ctx, k8sClient, ClickHouseNamespace, "app.kubernetes.io/name=clickhouse")

		if kafkaReady {
			kafkaReadyCount++
		} else {
			kafkaReadyCount = 0
		}

		if clickhouseReady {
			clickhouseReadyCount++
		} else {
			clickhouseReadyCount = 0
		}

		// Require pods to be ready for at least 3 consecutive checks (6 seconds)
		if kafkaReadyCount >= 3 && clickhouseReadyCount >= 3 {
			fmt.Println("✅ Kafka and ClickHouse are ready")
			return nil
		}

		// Progress and debug hints at interval
		if time.Since(lastProgress) >= WaitProgressInterval {
			lastProgress = time.Now()
			elapsed := time.Since(startTime).Round(time.Second)
			status := []string{}
			if kafkaReadyCount < 3 {
				status = append(status, "Kafka")
			}
			if clickhouseReadyCount < 3 {
				status = append(status, "ClickHouse")
			}
			fmt.Printf("⏳ Waiting for %v pods (%s elapsed). Check: kubectl get pods -n kafka -n clickhouse\n", status, elapsed)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			// Continue polling
		}
	}
}

// waitForPipelineReady polls the pipeline health endpoint until overall_status is "Running" or "Failed".
// Returns nil when running, or an error when failed or timeout.
func waitForPipelineReady(ctx context.Context, apiClient *demo.APIClient, pipelineID string) error {
	const (
		pollInterval = 3 * time.Second
		timeout      = 5 * time.Minute
	)
	deadline := time.Now().Add(timeout)
	startTime := time.Now()
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	fmt.Print("\n⏳ Waiting for pipeline to reach running state")
	pollCount := 0
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for pipeline to become ready after %v", timeout)
		}
		health, err := apiClient.GetPipelineHealth(ctx, pipelineID)
		if err != nil {
			if demo.IsConnectionError(err) {
				// Transient; keep polling
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-ticker.C:
					continue
				}
			}
			return fmt.Errorf("failed to get pipeline health: %w", err)
		}
		status := strings.TrimSpace(strings.ToLower(health.OverallStatus))
		switch status {
		case "running":
			fmt.Printf("\n✅ Pipeline is running (took %s)\n", time.Since(startTime).Round(time.Second))
			return nil
		case "failed":
			return fmt.Errorf("pipeline entered failed state (pipeline_id=%s). Check GlassFlow UI or logs for details", pipelineID)
		default:
			// Pending, starting, etc. - keep polling
			pollCount++
			if pollCount%10 == 0 {
				fmt.Print(".")
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			// continue
		}
	}
}

// checkPodsReady checks if all pods matching the selector in the given namespace are ready
func (i *Manager) checkPodsReady(ctx context.Context, client kubernetes.Interface, namespace, selector string) bool {
	pods, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: selector,
	})
	if err != nil {
		return false
	}

	if len(pods.Items) == 0 {
		return false
	}

	// Check if all pods are ready
	for _, pod := range pods.Items {
		// Skip pods that are being terminated
		if pod.DeletionTimestamp != nil {
			continue
		}

		// Check if pod is in Ready condition
		ready := false
		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.PodReady {
				ready = condition.Status == corev1.ConditionTrue
				break
			}
		}

		if !ready {
			return false
		}
	}

	return true
}

func (i *Manager) setupDemo(ctx context.Context, portMapping *k8s.PortMapping) error {
	if portMapping == nil {
		return fmt.Errorf("port mapping is required for demo setup")
	}

	fmt.Println("🎬 Setting up demo pipeline...")

	// Load demo files
	pipelineJSONPath, err := demo.LoadPipelineRequestPath()
	if err != nil {
		return fmt.Errorf("failed to load pipeline request: %w", err)
	}
	// Pipeline ID from request JSON (e.g. demo-pipeline-qasxjh) is used for health polling
	pipelineIDFromRequest, _ := demo.GetPipelineIDFromRequest(pipelineJSONPath)

	clickhouseSQLPath, err := demo.LoadClickHouseSQLPath()
	if err != nil {
		return fmt.Errorf("failed to load ClickHouse SQL: %w", err)
	}

	// Wait for Kafka and ClickHouse to be ready before setting up demo
	fmt.Println("⏳ Waiting for Kafka and ClickHouse to be ready...")
	if err := i.waitForKafkaAndClickHouse(ctx); err != nil {
		return fmt.Errorf("failed to wait for services to be ready: %w", err)
	}

	// Step 1: Create ClickHouse table
	clickhousePassword := DemoClickHousePassword
	if secret, err := i.k8sManager.GetSecret(ctx, "clickhouse", ClickHouseNamespace); err == nil {
		if pwd, ok := secret["admin-password"]; ok && len(pwd) > 0 {
			clickhousePassword = string(pwd)
		}
	}

	clickhouseURL := fmt.Sprintf("http://localhost:%d", portMapping.ClickHouseHTTP)
	clickhouseClient := demo.NewClickHouseClientWithAuth(clickhouseURL, DemoClickHouseUsername, clickhousePassword)
	if err := clickhouseClient.CreateTable(ctx, clickhouseSQLPath); err != nil {
		return fmt.Errorf("failed to create ClickHouse table: %w", err)
	}

	// // Step 2: Verify Kafka coordinator is ready and create topic before creating pipeline
	// fmt.Println("⏳ Verifying Kafka coordinator is ready...")
	// if err := i.waitForKafkaCoordinator(ctx); err != nil {
	// 	fmt.Printf("⚠️  Warning: Kafka coordinator check failed: %v\n", err)
	// 	fmt.Println("💡 Continuing with pipeline creation - will retry if coordinator not ready")
	// }

	// Step 3: Create pipeline via GlassFlow API with continuous polling
	// This handles transient errors like coordinator not ready, API not ready, etc.
	kafkaPassword := DemoKafkaPassword

	apiURL := fmt.Sprintf("http://localhost:%d", portMapping.GlassFlowAPI)
	apiClient := demo.NewAPIClient(apiURL)

	// Poll continuously with exponential backoff (capped at 10s)
	// Total timeout: ~5 minutes (with exponential backoff)
	maxTimeout := 5 * time.Minute
	startTime := time.Now()
	retryDelay := 2 * time.Second
	maxRetryDelay := 10 * time.Second
	attempt := 0

	fmt.Print("⏳ Creating pipeline via GlassFlow API")
	for {
		attempt++

		// Check timeout
		elapsed := time.Since(startTime)
		if elapsed > maxTimeout {
			fmt.Printf("\n")
			return fmt.Errorf("pipeline creation timed out after %v (attempted %d times)", maxTimeout, attempt)
		}

		// Try to create pipeline
		pipelineID, err := apiClient.CreatePipeline(ctx, pipelineJSONPath, DemoKafkaUsername, kafkaPassword, DemoClickHouseUsername, DemoClickHousePassword)
		if err == nil {
			// Success
			fmt.Printf("\n✅ Pipeline created successfully (after %d attempts, %v elapsed)\n", attempt, elapsed.Round(time.Second))
			// Use pipeline ID from API response, or from request JSON (e.g. demo-pipeline-qasxjh) for health polling
			healthPollID := pipelineID
			if healthPollID == "" {
				healthPollID = pipelineIDFromRequest
			}
			if healthPollID != "" {
				if waitErr := waitForPipelineReady(ctx, apiClient, healthPollID); waitErr != nil {
					return waitErr
				}
			}
			break
		}

		// Check if it's a connection error - might indicate port forward is down
		if demo.IsConnectionError(err) {
			fmt.Printf("\n🔄 Connection error detected, restarting port forwarding for GlassFlow API...\n")
			if restartErr := k8s.RestartPortForwardForService(i.config.KubeContext, i.config.Namespace, "glassflow-api", portMapping.GlassFlowAPI, 8081); restartErr != nil {
				fmt.Printf("⚠️  Failed to restart port forwarding: %v\n", restartErr)
			} else {
				fmt.Printf("✅ Port forwarding restarted, retrying request...\n")
			}
			// Wait a bit longer after restarting port forward
			time.Sleep(2 * time.Second)
		}

		// Show polling indicator every few attempts to show it's still working
		if attempt%3 == 0 {
			fmt.Print(".")
		}

		// Wait before retry with exponential backoff (capped)
		time.Sleep(retryDelay)
		retryDelay *= 2
		if retryDelay > maxRetryDelay {
			retryDelay = maxRetryDelay
		}
	}

	// Step 4: Create Kafka producer deployment (runs continuously)
	k8sClient, err := i.k8sManager.GetKubernetesClient()
	if err != nil {
		return fmt.Errorf("failed to get k8s client: %w", err)
	}

	// Use deterministic Kafka password for producer deployment
	producerKafkaPassword := DemoKafkaPassword
	if err := demo.CreateKafkaProducerDeployment(
		ctx,
		k8sClient,
		i.config.Namespace,
		"kafka.kafka.svc.cluster.local:9092",
		"demo-events",
		DemoKafkaUsername,
		producerKafkaPassword,
		1, // Produce 1 message per second
	); err != nil {
		return fmt.Errorf("failed to create Kafka producer deployment: %w", err)
	}

	fmt.Println("✅ Demo pipeline setup complete!")
	fmt.Println("💡 Kafka producer is running continuously and sending messages to 'demo-events' topic")

	return nil
}
