package helm

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Manager struct {
	repoConfigPath string
	repoCachePath  string
	kubeconfig     string
	kubeContext    string
	config         *Config
}

type Config struct {
	Namespace    string
	Kubeconfig   string
	Context      string
	Repositories []Repository
	Verbose      bool
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
	ValuesFile      string                 // when set, passed as -f to helm (overrides Values)
	SetValues       map[string]string      // extra --set key=value pairs (applied on top of ValuesFile/Values)
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
	if h.kubeconfig != "" {
		args = append(args, "--kubeconfig", h.kubeconfig)
	}
	if h.kubeContext != "" {
		args = append(args, "--kube-context", h.kubeContext)
	}
	return args
}

// openLogFile returns a writer for helm output. In verbose mode, returns nil (use stdout).
// In quiet mode, appends to ~/.glassflow/install.log.
func (h *Manager) openLogFile() *os.File {
	if h.config.Verbose {
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	logDir := filepath.Join(home, ".glassflow")
	_ = os.MkdirAll(logDir, 0o755)
	f, err := os.OpenFile(filepath.Join(logDir, "install.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil
	}
	return f
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

	args := append(h.helmBaseArgs(), "repo", "add", repoConfig.Name, repoConfig.URL)
	cmd := exec.CommandContext(ctx, "helm", args...)
	if logFile := h.openLogFile(); logFile != nil {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
		defer logFile.Close()
	} else {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
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
	args := append(h.helmBaseArgs(), "repo", "update")
	cmd := exec.CommandContext(ctx, "helm", args...)
	if logFile := h.openLogFile(); logFile != nil {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
		defer logFile.Close()
	} else {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
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
	for k, v := range opts.SetValues {
		args = append(args, "--set", k+"="+v)
	}

	cmd := exec.CommandContext(ctx, "helm", args...)
	var outputBuf bytes.Buffer
	if logFile := h.openLogFile(); logFile != nil {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
		defer logFile.Close()
	} else {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	cmd.Env = h.helmEnv()

	if err := cmd.Run(); err != nil {
		// On failure, show captured output to help debugging
		if outputBuf.Len() > 0 {
			fmt.Fprint(os.Stderr, outputBuf.String())
		}
		return nil, fmt.Errorf("helm upgrade --install %s failed: %w", opts.ReleaseName, err)
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

	cmd := exec.CommandContext(ctx, "helm", args...)
	if logFile := h.openLogFile(); logFile != nil {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
		defer logFile.Close()
	} else {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	cmd.Env = h.helmEnv()

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("helm uninstall failed: %w", err)
	}
	return nil
}
