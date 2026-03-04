package helm

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Manager struct {
	repoConfigPath string
	repoCachePath   string
	kubeconfig      string
	kubeContext     string
	config          *Config
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
	Version         string // chart version; when empty, Helm uses latest
	ReleaseName     string
	Namespace       string
	Values          map[string]interface{} // used when ValuesFile is empty
	ValuesFile      string                // when set, passed as -f to helm (overrides Values)
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

func helmConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".glassflow", "helm"), nil
}

func NewManager(_ interface{}, config *Config) *Manager {
	repoConfigPath := ""
	repoCachePath := ""
	if base, err := helmConfigDir(); err == nil {
		repoConfigPath = filepath.Join(base, "repositories.yaml")
		repoCachePath = filepath.Join(base, "repository")
	}

	return &Manager{
		repoConfigPath: repoConfigPath,
		repoCachePath:  repoCachePath,
		kubeconfig:     config.Kubeconfig,
		kubeContext:    config.Context,
		config:         config,
	}
}

func (h *Manager) helmEnv() []string {
	env := os.Environ()
	if h.kubeconfig != "" {
		env = append(env, "KUBECONFIG="+h.kubeconfig)
	}
	if h.kubeContext != "" {
		env = append(env, "HELM_KUBECONTEXT="+h.kubeContext)
	}
	return env
}

func (h *Manager) helmBaseArgs() []string {
	args := []string{}
	if h.repoConfigPath != "" {
		args = append(args, "--repository-config", h.repoConfigPath, "--repository-cache", h.repoCachePath)
	}
	return args
}

func (h *Manager) AddRepository(ctx context.Context, repoConfig *Repository) error {
	if h.repoConfigPath == "" {
		return fmt.Errorf("helm config directory not available")
	}
	if err := os.MkdirAll(filepath.Dir(h.repoConfigPath), 0o755); err != nil {
		return fmt.Errorf("failed to create repository config directory: %w", err)
	}
	if err := os.MkdirAll(h.repoCachePath, 0o755); err != nil {
		return fmt.Errorf("failed to create repository cache directory: %w", err)
	}

	fmt.Printf("🔧 Running: helm repo add %s %s\n", repoConfig.Name, repoConfig.URL)

	args := append(h.helmBaseArgs(), "repo", "add", repoConfig.Name, repoConfig.URL)
	cmd := exec.CommandContext(ctx, "helm", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = h.helmEnv()

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("helm repo add failed: %w", err)
	}
	return nil
}

// UpdateRepositories refreshes the local cache of all configured Helm repos so installs use the latest chart index.
func (h *Manager) UpdateRepositories(ctx context.Context) error {
	if h.repoConfigPath == "" {
		return nil // no custom config, nothing to update
	}
	fmt.Printf("🔧 Running: helm repo update\n")
	args := append(h.helmBaseArgs(), "repo", "update")
	cmd := exec.CommandContext(ctx, "helm", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = h.helmEnv()
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("helm repo update failed: %w", err)
	}
	return nil
}

func (h *Manager) InstallChart(ctx context.Context, opts *InstallOptions) (*Release, error) {
	var valuesPath string
	if opts.ValuesFile != "" {
		valuesPath = opts.ValuesFile
	} else {
		valuesYAML, err := yaml.Marshal(opts.Values)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal values: %w", err)
		}
		valuesFile, err := os.CreateTemp("", "glassflow-helm-values-*.yaml")
		if err != nil {
			return nil, fmt.Errorf("failed to create values file: %w", err)
		}
		valuesPath = valuesFile.Name()
		defer os.Remove(valuesPath)
		if _, err := valuesFile.Write(valuesYAML); err != nil {
			valuesFile.Close()
			return nil, fmt.Errorf("failed to write values file: %w", err)
		}
		if err := valuesFile.Close(); err != nil {
			return nil, fmt.Errorf("failed to close values file: %w", err)
		}
	}

	// helm upgrade --install treats install and upgrade the same; --create-namespace creates namespace if needed
	args := h.helmBaseArgs()
	args = append(args, "upgrade", "--install", opts.ReleaseName, opts.Chart,
		"--namespace", opts.Namespace,
		"-f", valuesPath,
		"--timeout", fmt.Sprintf("%ds", opts.Timeout),
	)
	if opts.Version != "" {
		args = append(args, "--version", opts.Version)
	}
	if opts.CreateNamespace {
		args = append(args, "--create-namespace")
	}
	if opts.Wait {
		args = append(args, "--wait")
	}

	fmt.Printf("🔧 Running: helm upgrade --install %s %s --namespace %s -f %s --timeout %ds\n",
		opts.ReleaseName, opts.Chart, opts.Namespace, valuesPath, opts.Timeout)

	cmd := exec.CommandContext(ctx, "helm", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = h.helmEnv()

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("helm upgrade --install failed: %w", err)
	}

	return &Release{
		Name:      opts.ReleaseName,
		Namespace: opts.Namespace,
		Chart:     opts.Chart,
	}, nil
}

func (h *Manager) UninstallChart(ctx context.Context, opts *UninstallOptions) error {
	args := h.helmBaseArgs()
	args = append(args, "uninstall", opts.ReleaseName,
		"--namespace", opts.Namespace,
		"--timeout", fmt.Sprintf("%ds", opts.Timeout),
	)
	if opts.Wait {
		args = append(args, "--wait")
	}

	fmt.Printf("🔧 Running: helm uninstall %s --namespace %s --timeout %ds\n",
		opts.ReleaseName, opts.Namespace, opts.Timeout)

	cmd := exec.CommandContext(ctx, "helm", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = h.helmEnv()

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("helm uninstall failed: %w", err)
	}
	return nil
}
