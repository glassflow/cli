package helm

import (
	"context"
	"fmt"
	"time"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/kube"
	"helm.sh/helm/v3/pkg/repo"
	"k8s.io/client-go/kubernetes"
)

type Manager struct {
	client   kubernetes.Interface
	settings *cli.EnvSettings
	config   *Config
}

type Config struct {
	Namespace    string
	Kubeconfig   string
	Context      string
	Repositories []Repository
}

type Repository struct {
	Name string
	URL  string
}

type InstallOptions struct {
	Chart           string
	ReleaseName     string
	Namespace       string
	Values          map[string]interface{}
	CreateNamespace bool
	Wait            bool
	Timeout         int
}

type UninstallOptions struct {
	ReleaseName string
	Namespace   string
	Wait        bool
	Timeout     int
}

type Release struct {
	Name      string
	Namespace string
	Status    string
	Version   string
	Chart     string
}

func NewManager(client kubernetes.Interface, config *Config) *Manager {
	settings := cli.New()
	settings.KubeConfig = config.Kubeconfig
	settings.KubeContext = config.Context

	return &Manager{
		client:   client,
		settings: settings,
		config:   config,
	}
}

func (h *Manager) AddRepository(ctx context.Context, repoConfig *Repository) error {
	// Print the Helm command being executed
	fmt.Printf("🔧 Running: helm repo add %s %s\n", repoConfig.Name, repoConfig.URL)

	chartRepo, err := repo.NewChartRepository(&repo.Entry{
		Name: repoConfig.Name,
		URL:  repoConfig.URL,
	}, getter.All(h.settings))
	if err != nil {
		return fmt.Errorf("failed to create chart repository: %w", err)
	}

	_, err = chartRepo.DownloadIndexFile()
	if err != nil {
		return fmt.Errorf("failed to download index file: %w", err)
	}

	// TODO: Add repository to Helm's repository list
	return nil
}

func (h *Manager) InstallChart(ctx context.Context, opts *InstallOptions) (*Release, error) {
	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(h.settings.RESTClientGetter(), opts.Namespace, "secret", func(format string, v ...interface{}) {
		// TODO: Implement logging
	}); err != nil {
		return nil, fmt.Errorf("failed to initialize action config: %w", err)
	}

	// Fix for namespace issue: When actionConfig.Init is called it sets up the driver with the default namespace.
	// We need to change the namespace to honor the release namespace.
	// https://github.com/helm/helm/issues/9171
	if kubeClient, ok := actionConfig.KubeClient.(*kube.Client); ok {
		kubeClient.Namespace = opts.Namespace
	}

	installAction := action.NewInstall(actionConfig)
	installAction.ReleaseName = opts.ReleaseName
	installAction.Namespace = opts.Namespace
	installAction.CreateNamespace = opts.CreateNamespace
	installAction.Wait = opts.Wait
	installAction.Timeout = time.Duration(opts.Timeout) * time.Second

	// Print the Helm command being executed
	fmt.Printf("🔧 Running: helm install %s %s --namespace %s --create-namespace --wait --timeout %ds\n",
		opts.ReleaseName, opts.Chart, opts.Namespace, opts.Timeout)

	chartPath, err := installAction.LocateChart(opts.Chart, h.settings)
	if err != nil {
		return nil, fmt.Errorf("failed to locate chart: %w", err)
	}

	chart, err := loader.Load(chartPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load chart: %w", err)
	}

	release, err := installAction.RunWithContext(ctx, chart, opts.Values)
	if err != nil {
		return nil, fmt.Errorf("failed to install chart: %w", err)
	}

	return &Release{
		Name:      release.Name,
		Namespace: release.Namespace,
		Status:    release.Info.Status.String(),
		Version:   fmt.Sprintf("%d", release.Version),
		Chart:     release.Chart.Metadata.Name,
	}, nil
}

func (h *Manager) UninstallChart(ctx context.Context, opts *UninstallOptions) error {
	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(h.settings.RESTClientGetter(), opts.Namespace, "secret", func(format string, v ...interface{}) {
		// TODO: Implement logging
	}); err != nil {
		return fmt.Errorf("failed to initialize action config: %w", err)
	}

	uninstallAction := action.NewUninstall(actionConfig)
	uninstallAction.Wait = opts.Wait
	uninstallAction.Timeout = time.Duration(opts.Timeout) * time.Second

	// Print the Helm command being executed
	fmt.Printf("🔧 Running: helm uninstall %s --namespace %s --wait --timeout %ds\n",
		opts.ReleaseName, opts.Namespace, opts.Timeout)

	_, err := uninstallAction.Run(opts.ReleaseName)
	if err != nil {
		return fmt.Errorf("failed to uninstall chart: %w", err)
	}

	return nil
}
