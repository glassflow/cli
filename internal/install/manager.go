package install

import (
	"context"
	"fmt"
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

// Deterministic credentials for demo setup
const (
	// Kafka credentials
	DemoKafkaUsername = "user1"
	DemoKafkaPassword = "glassflow-demo-password"

	// ClickHouse credentials
	DemoClickHouseUsername = "default"
	DemoClickHousePassword = "glassflow-demo-password"
)

type Manager struct {
	helmManager *helm.Manager
	k8sManager  *k8s.Manager
	config      *Config
}

type Config struct {
	Namespace string
	Demo      bool
	Charts    *config.ChartsConfig
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
			Namespace:   i.config.Namespace,
			Wait:        true,
		}); err != nil {
			fmt.Printf("Warning: failed to uninstall Kafka: %v\n", err)
		}

		if err := i.helmManager.UninstallChart(ctx, &helm.UninstallOptions{
			ReleaseName: "clickhouse",
			Namespace:   i.config.Namespace,
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
	// Reduced resource values for local Kind cluster MVP
	// Based on: https://raw.githubusercontent.com/glassflow/charts/refs/heads/main/charts/glassflow-etl/values.yaml
	values := map[string]interface{}{
		"glassflow-operator": map[string]interface{}{
			"glassflowComponents": map[string]interface{}{
				"ingestor": map[string]interface{}{
					"resources": map[string]interface{}{
						"requests": map[string]interface{}{
							"cpu":    "250m",
							"memory": "256Mi",
						},
						"limits": map[string]interface{}{
							"cpu":    "500m",
							"memory": "512Mi",
						},
					},
				},
				"join": map[string]interface{}{
					"resources": map[string]interface{}{
						"requests": map[string]interface{}{
							"cpu":    "250m",
							"memory": "256Mi",
						},
						"limits": map[string]interface{}{
							"cpu":    "500m",
							"memory": "512Mi",
						},
					},
				},
				"sink": map[string]interface{}{
					"resources": map[string]interface{}{
						"requests": map[string]interface{}{
							"cpu":    "250m",
							"memory": "256Mi",
						},
						"limits": map[string]interface{}{
							"cpu":    "500m",
							"memory": "512Mi",
						},
					},
				},
			},
		},
		"nats": map[string]interface{}{
			"config": map[string]interface{}{
				"replicas": 2, // Reduced from 3 for MVP
				// Note: PVC size cannot be changed after StatefulSet creation,
				// so we don't override it here to allow upgrades
			},
			"resources": map[string]interface{}{
				"requests": map[string]interface{}{
					"memory": "512Mi", // Reduced from 2Gi
					"cpu":    "250m",  // Reduced from 500m
				},
				"limits": map[string]interface{}{
					"memory": "1Gi",  // Reduced from 4Gi
					"cpu":    "500m", // Reduced from 1000m
				},
			},
		},
	}

	_, err := i.helmManager.InstallChart(ctx, &helm.InstallOptions{
		Chart:           i.config.Charts.GlassFlow.Chart,
		ReleaseName:     "glassflow",
		Namespace:       i.config.Namespace,
		Values:          values,
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

	// Delete existing secret if it exists
	_ = k8sClient.CoreV1().Secrets(i.config.Namespace).Delete(ctx, secretName, metav1.DeleteOptions{})

	// Create secret with deterministic password and Helm labels for ownership
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: i.config.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/instance":   releaseName,
				"app.kubernetes.io/managed-by": "Helm",
				"app.kubernetes.io/name":       "kafka",
				"helm.sh/chart":                "kafka-32.4.3",
			},
			Annotations: map[string]string{
				"meta.helm.sh/release-name":      releaseName,
				"meta.helm.sh/release-namespace": i.config.Namespace,
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
	if _, createErr := k8sClient.CoreV1().Secrets(i.config.Namespace).Create(ctx, secret, metav1.CreateOptions{}); createErr != nil {
		fmt.Printf("⚠️  Warning: Failed to create Kafka secret: %v\n", createErr)
	} else {
		fmt.Printf("✅ Created Kafka secret with deterministic password\n")
	}

	// Set deterministic credentials and single replica for local dev
	// Optimized for fast demo startup
	values := map[string]interface{}{
		"replicaCount": 1, // Single replica for local dev
		"controller": map[string]interface{}{
			"replicaCount": 1, // Single controller for local dev (KRaft mode)
			// Reduce controller resources for faster startup
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
		"persistence": map[string]interface{}{
			"enabled": false, // Use emptyDir for local dev
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
		// Reduce resource usage for local dev
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
		// Optimize JVM heap size for faster startup
		"extraEnvVars": []map[string]interface{}{
			{
				"name":  "KAFKA_HEAP_OPTS",
				"value": "-Xmx512M -Xms512M", // Match memory request for faster JVM startup
			},
			// Reduce log retention for demo (1 hour instead of 7 days)
			{
				"name":  "KAFKA_LOG_RETENTION_HOURS",
				"value": "1",
			},
			{
				"name":  "KAFKA_LOG_RETENTION_BYTES",
				"value": "1073741824", // 1GB max retention
			},
			{
				"name":  "KAFKA_LOG_SEGMENT_BYTES",
				"value": "104857600", // 100MB segments (smaller = faster cleanup)
			},
		},
		// Disable metrics for demo to reduce resource usage
		"metrics": map[string]interface{}{
			"enabled": false,
		},
		// Disable JMX for demo
		"jmx": map[string]interface{}{
			"enabled": false,
		},
		// Optimize startup/readiness probes for faster detection
		"startupProbe": map[string]interface{}{
			"enabled":             true,
			"initialDelaySeconds": 10, // Reduced from default 30s
			"periodSeconds":       5,
			"timeoutSeconds":      5,
			"failureThreshold":    12, // 60s total timeout
		},
		"readinessProbe": map[string]interface{}{
			"initialDelaySeconds": 5, // Reduced from default 10s
			"periodSeconds":       5,
			"timeoutSeconds":      3,
		},
		"livenessProbe": map[string]interface{}{
			"initialDelaySeconds": 30, // Keep reasonable for liveness
			"periodSeconds":       10,
			"timeoutSeconds":      5,
		},
	}

	_, err = i.helmManager.InstallChart(ctx, &helm.InstallOptions{
		Chart:           i.config.Charts.Kafka.Chart,
		ReleaseName:     "kafka",
		Namespace:       i.config.Namespace,
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
		ReleaseName:     "clickhouse",
		Namespace:       i.config.Namespace,
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

	services := []string{"glassflow-api", "glassflow-ui", "clickhouse"}
	timeout := 5 * time.Minute
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for services to be ready after %v", timeout)
		}

		allReady := true
		for _, svcName := range services {
			endpoints, err := k8sClient.CoreV1().Endpoints(i.config.Namespace).Get(ctx, svcName, metav1.GetOptions{})
			if err != nil {
				allReady = false
				break
			}
			// Check if service has at least one ready endpoint
			hasReadyEndpoint := false
			for _, subset := range endpoints.Subsets {
				if len(subset.Addresses) > 0 {
					hasReadyEndpoint = true
					break
				}
			}
			if !hasReadyEndpoint {
				allReady = false
				break
			}
		}

		if allReady {
			fmt.Println("✅ Services are ready")
			return nil
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

	timeout := 5 * time.Minute
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for Kafka and ClickHouse to be ready after %v", timeout)
		}

		// Check Kafka readiness
		kafkaReady := i.checkPodsReady(ctx, k8sClient, "app.kubernetes.io/name=kafka")
		// Check ClickHouse readiness
		clickhouseReady := i.checkPodsReady(ctx, k8sClient, "app.kubernetes.io/name=clickhouse")

		if kafkaReady && clickhouseReady {
			fmt.Println("✅ Kafka and ClickHouse are ready")
			return nil
		}

		// Wait before next check
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			// Continue polling
		}
	}
}

// checkPodsReady checks if all pods matching the selector are ready
func (i *Manager) checkPodsReady(ctx context.Context, client kubernetes.Interface, selector string) bool {
	pods, err := client.CoreV1().Pods(i.config.Namespace).List(ctx, metav1.ListOptions{
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

	clickhouseSQLPath, err := demo.LoadClickHouseSQLPath()
	if err != nil {
		return fmt.Errorf("failed to load ClickHouse SQL: %w", err)
	}

	// Wait for Kafka and ClickHouse to be ready before setting up demo
	fmt.Println("⏳ Waiting for Kafka and ClickHouse to be ready...")
	if err := i.waitForKafkaAndClickHouse(ctx); err != nil {
		return fmt.Errorf("failed to wait for services to be ready: %w", err)
	}

	// Note: Port forwarding readiness is already checked in SetupPortForwarding()

	// Step 1: Create ClickHouse table
	// Use deterministic ClickHouse password (try secret first, fallback to constant)
	clickhousePassword := DemoClickHousePassword
	if secret, err := i.k8sManager.GetSecret(ctx, "clickhouse", i.config.Namespace); err == nil {
		if pwd, ok := secret["admin-password"]; ok && len(pwd) > 0 {
			clickhousePassword = string(pwd)
		}
	}

	clickhouseURL := fmt.Sprintf("http://localhost:%d", portMapping.ClickHouseHTTP)
	clickhouseClient := demo.NewClickHouseClientWithAuth(clickhouseURL, DemoClickHouseUsername, clickhousePassword)
	if err := clickhouseClient.CreateTable(ctx, clickhouseSQLPath); err != nil {
		return fmt.Errorf("failed to create ClickHouse table: %w", err)
	}

	// Step 2: Create pipeline via GlassFlow API
	// Use deterministic Kafka password (Bitnami chart may generate random passwords in secret)
	// Use our constant password instead of reading from secret to ensure consistency
	kafkaPassword := DemoKafkaPassword

	apiURL := fmt.Sprintf("http://localhost:%d", portMapping.GlassFlowAPI)
	apiClient := demo.NewAPIClient(apiURL)
	if err := apiClient.CreatePipeline(ctx, pipelineJSONPath, DemoKafkaUsername, kafkaPassword, DemoClickHouseUsername, DemoClickHousePassword); err != nil {
		return fmt.Errorf("failed to create pipeline: %w", err)
	}

	// Step 3: Create Kafka producer deployment (runs continuously)
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
		"kafka.glassflow.svc.cluster.local:9092",
		"demo-events",
		DemoKafkaUsername,
		producerKafkaPassword,
		100, // Produce 1 message per second
	); err != nil {
		return fmt.Errorf("failed to create Kafka producer deployment: %w", err)
	}

	fmt.Println("✅ Demo pipeline setup complete!")
	fmt.Println("💡 Kafka producer is running continuously and sending messages to 'demo-events' topic")

	return nil
}
