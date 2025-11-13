package k8s

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/kind/pkg/cluster"
)

type Manager struct {
	client     kubernetes.Interface
	kindClient *cluster.Provider
	config     *Config
}

type Config struct {
	ClusterName string
	Namespace   string
}

type ClusterStatus struct {
	Name   string
	Status string
}

func NewManager(config *Config) *Manager {
	kindClient := cluster.NewProvider()

	return &Manager{
		kindClient: kindClient,
		config:     config,
	}
}

func (m *Manager) CreateCluster(ctx context.Context) error {
	// Create Kind cluster with basic configuration
	// Use default Kubernetes version (Kind will use the latest supported version)
	return m.kindClient.Create(m.config.ClusterName)
}

func (m *Manager) DeleteCluster(ctx context.Context) error {
	return m.kindClient.Delete(m.config.ClusterName, "")
}

func (m *Manager) GetClusterStatus(ctx context.Context) (*ClusterStatus, error) {
	clusters, err := m.kindClient.List()
	if err != nil {
		return nil, fmt.Errorf("failed to list clusters: %w", err)
	}

	for _, cluster := range clusters {
		if cluster == m.config.ClusterName {
			return &ClusterStatus{
				Name:   cluster,
				Status: "Running",
			}, nil
		}
	}

	return &ClusterStatus{
		Name:   m.config.ClusterName,
		Status: "Not Found",
	}, nil
}

func (m *Manager) GetKubernetesClient() (kubernetes.Interface, error) {
	if m.client != nil {
		return m.client, nil
	}

	restConfig, err := m.GetRESTConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get kubeconfig: %w", err)
	}

	client, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create k8s client: %w", err)
	}

	m.client = client
	return client, nil
}

// GetRESTConfig returns a REST config for connecting to the current cluster.
func (m *Manager) GetRESTConfig() (*rest.Config, error) {
	kubeconfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{},
	)
	restConfig, err := kubeconfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get kubeconfig: %w", err)
	}
	return restConfig, nil
}

// WaitForClusterReady polls the Kubernetes API until nodes are Ready or the timeout elapses.
func (m *Manager) WaitForClusterReady(ctx context.Context, timeout time.Duration) error {
	fmt.Println("⏳ Waiting for Kubernetes API and nodes to become Ready...")
	deadline := time.Now().Add(timeout)
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("cluster did not become Ready within %s", timeout.String())
		}
		client, err := m.GetKubernetesClient()
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}
		nodes, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
		if err != nil || len(nodes.Items) == 0 {
			time.Sleep(2 * time.Second)
			continue
		}
		allReady := true
		for _, n := range nodes.Items {
			if !isNodeReady(&n) {
				allReady = false
				break
			}
		}
		if allReady {
			fmt.Println("✅ Kubernetes nodes are Ready")
			return nil
		}
		time.Sleep(2 * time.Second)
	}
}

func isNodeReady(n *corev1.Node) bool {
	for _, c := range n.Status.Conditions {
		if c.Type == corev1.NodeReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}

// GetSecret retrieves a Kubernetes secret and returns its data as a map
func (m *Manager) GetSecret(ctx context.Context, secretName, namespace string) (map[string][]byte, error) {
	client, err := m.GetKubernetesClient()
	if err != nil {
		return nil, fmt.Errorf("failed to get k8s client: %w", err)
	}

	secret, err := client.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get secret %s/%s: %w", namespace, secretName, err)
	}

	return secret.Data, nil
}
