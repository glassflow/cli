package k8s

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type portForwardEntry struct {
	PID     int    `json:"pid"`
	Service string `json:"service"`
}

type portForwardState struct {
	Entries []portForwardEntry `json:"entries"`
}

func pfStatePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home dir: %w", err)
	}
	dir := filepath.Join(home, ".glassflow")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create state dir: %w", err)
	}
	return filepath.Join(dir, "portforwards.json"), nil
}

func loadPF() (*portForwardState, error) {
	path, err := pfStatePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &portForwardState{Entries: []portForwardEntry{}}, nil
		}
		return nil, fmt.Errorf("failed to read state file: %w", err)
	}
	var st portForwardState
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("failed to parse state file: %w", err)
	}
	return &st, nil
}

func savePF(st *portForwardState) error {
	path, err := pfStatePath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode state: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("failed to write state file: %w", err)
	}
	return nil
}

func clearPF() error {
	path, err := pfStatePath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove state file: %w", err)
	}
	return nil
}

func checkKubectl() error {
	cmd := exec.Command("kubectl", "version", "--client")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("kubectl not found or not working. please install kubectl and ensure it is in your PATH: %w", err)
	}
	return nil
}

// IsPortAvailable checks if a port is available for binding
func IsPortAvailable(port int) bool {
	ln, err := net.Listen("tcp", ":"+strconv.Itoa(port))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

func isPortAvailable(port int) bool {
	return IsPortAvailable(port)
}

func findAvailablePort(preferredPort int) int {
	for p := preferredPort; p < preferredPort+100; p++ {
		if isPortAvailable(p) {
			return p
		}
	}
	return 0
}

// PortMapping holds the actual port mappings for forwarded services
type PortMapping struct {
	GlassFlowUI    int
	GlassFlowAPI   int
	ClickHouseHTTP int
}

// SetupPortForwarding starts kubectl port-forward for UI, API, and ClickHouse.
// If kubectl missing, prints manual instructions including context.
// Returns port mappings for use by demo code.
func SetupPortForwarding(kubeContext string) (*PortMapping, error) {
	if err := checkKubectl(); err != nil {
		fmt.Println("⚠️  kubectl not found. Please port-forward manually with:")
		ctxFlag := ""
		if kubeContext != "" {
			ctxFlag = fmt.Sprintf(" --context %s", kubeContext)
		}
		fmt.Printf("   kubectl%s -n glassflow port-forward service/glassflow-api 30180:8081\n", ctxFlag)
		fmt.Printf("   kubectl%s -n glassflow port-forward service/glassflow-ui 30080:8080\n", ctxFlag)
		fmt.Printf("   kubectl%s -n glassflow port-forward service/clickhouse 30090:8123\n", ctxFlag)
		return nil, nil
	}

	services := map[string]struct {
		preferredPort int
		targetPort    int
		name          string
	}{
		"glassflow-api": {30180, 8081, "GlassFlow API"}, // API first (higher priority)
		"glassflow-ui":  {30080, 8080, "GlassFlow UI"},
		"clickhouse":    {30090, 8123, "ClickHouse HTTP"},
	}

	actual := make(map[string]int)
	st, _ := loadPF()

	// Start port forwarding with retry logic for services that aren't ready yet
	for svc, cfg := range services {
		port := findAvailablePort(cfg.preferredPort)
		if port == 0 {
			return nil, fmt.Errorf("no available ports found for %s (tried %d-%d)", cfg.name, cfg.preferredPort, cfg.preferredPort+99)
		}
		actual[svc] = port
		mapping := fmt.Sprintf("%d:%d", port, cfg.targetPort)
		args := []string{"port-forward", "-n", "glassflow", "service/" + svc, mapping}
		if kubeContext != "" {
			args = append([]string{"--context", kubeContext}, args...)
		}

		// Try to start port forwarding (may fail if service not ready, we'll retry)
		cmd := exec.Command("kubectl", args...)
		// Redirect both stdout and stderr to /dev/null to suppress all output
		devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
		if err == nil {
			cmd.Stdout = devNull
			cmd.Stderr = devNull
		} else {
			cmd.Stdout = nil
			cmd.Stderr = nil // Fallback if /dev/null can't be opened
		}
		if err := cmd.Start(); err != nil {
			// Don't fail immediately, we'll retry in the wait loop
			fmt.Printf("⚠️  Failed to start port forwarding for %s (will retry): %v\n", cfg.name, err)
		} else if cmd.Process != nil {
			st.Entries = append(st.Entries, portForwardEntry{PID: cmd.Process.Pid, Service: svc})
			_ = savePF(st)
		}
	}

	fmt.Printf("🔗 Port forwarding established:\n")
	if p, ok := actual["glassflow-ui"]; ok {
		fmt.Printf("   🌐 GlassFlow UI: http://localhost:%d\n", p)
	}
	if p, ok := actual["glassflow-api"]; ok {
		fmt.Printf("   🔌 GlassFlow API: http://localhost:%d\n", p)
	} else {
		return nil, fmt.Errorf("failed to establish port forwarding for GlassFlow API - port may be in use")
	}
	if p, ok := actual["clickhouse"]; ok {
		fmt.Printf("   🗄️  ClickHouse HTTP: http://localhost:%d\n", p)
	}

	mapping := &PortMapping{
		GlassFlowUI:    actual["glassflow-ui"],
		GlassFlowAPI:   actual["glassflow-api"],
		ClickHouseHTTP: actual["clickhouse"],
	}

	// Wait for port forwards to be ready with retry logic
	fmt.Println("⏳ Waiting for port forwarding to be ready...")
	// Increase timeout to 5 minutes and make failures non-fatal
	if err := waitForPortForwardsWithRetry(kubeContext, services, actual, mapping, 5*time.Minute); err != nil {
		// Log warning but don't fail - services are accessible via DNS within cluster
		fmt.Printf("⚠️  Warning: Port forwarding not fully ready: %v\n", err)
		fmt.Println("💡 Demo setup will continue - services are accessible via DNS within the cluster")
		// Only fail if GlassFlow API port-forward failed (critical for demo setup)
		if !isPortListening(mapping.GlassFlowAPI) {
			return nil, fmt.Errorf("critical: GlassFlow API port-forward failed - demo setup requires API access")
		}
	}

	return mapping, nil
}

// waitForPortForwardsWithRetry waits for port forwards to be ready, retrying failed ones
func waitForPortForwardsWithRetry(kubeContext string, services map[string]struct {
	preferredPort int
	targetPort    int
	name          string
}, actual map[string]int, mapping *PortMapping, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ports := []struct {
		name       string
		port       int
		service    string
		targetPort int
	}{
		{"GlassFlow API", mapping.GlassFlowAPI, "glassflow-api", 8081},
		{"GlassFlow UI", mapping.GlassFlowUI, "glassflow-ui", 8080},
		{"ClickHouse HTTP", mapping.ClickHouseHTTP, "clickhouse", 8123},
	}

	st, _ := loadPF()
	lastRetryTime := time.Now()
	retryInterval := 5 * time.Second
	lastPodCheckTime := time.Now()
	podCheckInterval := 3 * time.Second

	for {
		if time.Now().After(deadline) {
			// Return error with details about which ports failed
			failedPorts := []string{}
			for _, p := range ports {
				if !isPortListening(p.port) {
					failedPorts = append(failedPorts, fmt.Sprintf("%s (port %d)", p.name, p.port))
				}
			}
			if len(failedPorts) > 0 {
				return fmt.Errorf("timeout waiting for port forwarding after %v - failed ports: %v", timeout, failedPorts)
			}
			return fmt.Errorf("timeout waiting for port forwarding after %v", timeout)
		}

		allReady := true
		needsRetry := false

		// Check if pods are ready before retrying (to avoid unnecessary retries)
		podsReady := true
		if time.Since(lastPodCheckTime) >= podCheckInterval {
			lastPodCheckTime = time.Now()
			for _, p := range ports {
				if !isPortListening(p.port) {
					// Check if pods for this service are ready
					if !areServicePodsReady(kubeContext, p.service) {
						podsReady = false
						break
					}
				}
			}
		}

		for _, p := range ports {
			if !isPortListening(p.port) {
				allReady = false
				// Only retry if pods are ready and enough time has passed
				if podsReady && time.Since(lastRetryTime) >= retryInterval {
					needsRetry = true
				}
			}
		}

		if allReady {
			fmt.Println("✅ Port forwarding is ready")
			return nil
		}

		// Retry failed port forwards (only if pods are ready)
		if needsRetry && podsReady {
			lastRetryTime = time.Now()
			for _, p := range ports {
				if !isPortListening(p.port) {
					fmt.Printf("🔄 Retrying port forward for %s...\n", p.name)
					// Kill any existing process for this service
					newEntries := []portForwardEntry{}
					for _, entry := range st.Entries {
						if entry.Service == p.service {
							if proc, err := os.FindProcess(entry.PID); err == nil {
								_ = proc.Kill()
							}
							// Don't add to newEntries (effectively removing it)
						} else {
							newEntries = append(newEntries, entry)
						}
					}
					st.Entries = newEntries

					// Retry starting port forward
					mappingStr := fmt.Sprintf("%d:%d", p.port, p.targetPort)
					args := []string{"port-forward", "-n", "glassflow", "service/" + p.service, mappingStr}
					if kubeContext != "" {
						args = append([]string{"--context", kubeContext}, args...)
					}
					cmd := exec.Command("kubectl", args...)
					// Redirect both stdout and stderr to /dev/null to suppress all output
					devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
					if err == nil {
						cmd.Stdout = devNull
						cmd.Stderr = devNull
					} else {
						cmd.Stdout = nil
						cmd.Stderr = nil // Fallback if /dev/null can't be opened
					}
					if err := cmd.Start(); err == nil && cmd.Process != nil {
						st.Entries = append(st.Entries, portForwardEntry{PID: cmd.Process.Pid, Service: p.service})
						_ = savePF(st)
					} else {
						fmt.Printf("⚠️  Failed to restart port forward for %s: %v\n", p.name, err)
					}
				}
			}
		}

		time.Sleep(1 * time.Second)
	}
}

// areServicePodsReady checks if pods for a service are ready by checking service endpoints
func areServicePodsReady(kubeContext string, serviceName string) bool {
	args := []string{"get", "endpoints", "-n", "glassflow", serviceName, "-o", "jsonpath={.subsets[*].addresses[*].ip}"}
	if kubeContext != "" {
		args = append([]string{"--context", kubeContext}, args...)
	}
	cmd := exec.Command("kubectl", args...)
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	// Check if service has at least one ready endpoint (address)
	outputStr := strings.TrimSpace(string(output))
	return len(outputStr) > 0
}

// isPortListening checks if a port is listening (port forward is ready)
func isPortListening(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", port), 1*time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// RestartPortForwardForService restarts port forwarding for a specific service
// This is useful when a pod restarts and the port forward connection is lost
func RestartPortForwardForService(kubeContext string, serviceName string, port int, targetPort int) error {
	st, err := loadPF()
	if err != nil {
		return fmt.Errorf("failed to load port-forward state: %w", err)
	}

	// Kill any existing process for this service
	newEntries := []portForwardEntry{}
	for _, entry := range st.Entries {
		if entry.Service == serviceName {
			if proc, err := os.FindProcess(entry.PID); err == nil {
				_ = proc.Kill()
			}
			// Don't add to newEntries (effectively removing it)
		} else {
			newEntries = append(newEntries, entry)
		}
	}
	st.Entries = newEntries

	// Start new port forward
	mappingStr := fmt.Sprintf("%d:%d", port, targetPort)
	args := []string{"port-forward", "-n", "glassflow", "service/" + serviceName, mappingStr}
	if kubeContext != "" {
		args = append([]string{"--context", kubeContext}, args...)
	}

	cmd := exec.Command("kubectl", args...)
	// Redirect both stdout and stderr to /dev/null to suppress all output
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err == nil {
		cmd.Stdout = devNull
		cmd.Stderr = devNull
	} else {
		cmd.Stdout = nil
		cmd.Stderr = nil
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start port forward: %w", err)
	}

	if cmd.Process != nil {
		st.Entries = append(st.Entries, portForwardEntry{PID: cmd.Process.Pid, Service: serviceName})
		if err := savePF(st); err != nil {
			return fmt.Errorf("failed to save port-forward state: %w", err)
		}
	}

	// Give it a moment to establish the connection
	time.Sleep(1 * time.Second)
	return nil
}

// CleanupPortForwarding kills only the port-forwards started by this CLI using stored PIDs.
func CleanupPortForwarding(verbose bool) {
	st, err := loadPF()
	if err != nil {
		if verbose {
			fmt.Printf("(info) failed to load port-forward state: %v\n", err)
		}
		return
	}
	for _, e := range st.Entries {
		proc, perr := os.FindProcess(e.PID)
		if perr == nil {
			_ = proc.Kill()
		}
	}
	_ = clearPF()
}
