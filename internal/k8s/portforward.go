package k8s

import (
    "encoding/json"
    "fmt"
    "net"
    "os"
    "os/exec"
    "path/filepath"
    "strconv"
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

func isPortAvailable(port int) bool {
    ln, err := net.Listen("tcp", ":"+strconv.Itoa(port))
    if err != nil {
        return false
    }
    _ = ln.Close()
    return true
}

func findAvailablePort(preferredPort int) int {
    for p := preferredPort; p < preferredPort+100; p++ {
        if isPortAvailable(p) {
            return p
        }
    }
    return 0
}

// SetupPortForwarding starts kubectl port-forward for UI and API. If kubectl missing, prints manual instructions including context.
func SetupPortForwarding(kubeContext string) error {
    if err := checkKubectl(); err != nil {
        fmt.Println("⚠️  kubectl not found. Please port-forward manually with:")
        ctxFlag := ""
        if kubeContext != "" {
            ctxFlag = fmt.Sprintf(" --context %s", kubeContext)
        }
        fmt.Printf("   kubectl%s -n glassflow port-forward service/glassflow-ui 30080:8080\n", ctxFlag)
        fmt.Printf("   kubectl%s -n glassflow port-forward service/glassflow-api 30081:8081\n", ctxFlag)
        return nil
    }

    services := map[string]struct {
        preferredPort int
        targetPort    int
        name          string
    }{
        "glassflow-ui":  {30080, 8080, "GlassFlow UI"},
        "glassflow-api": {30081, 8081, "GlassFlow API"},
    }

    actual := make(map[string]int)
    st, _ := loadPF()

    for svc, cfg := range services {
        port := findAvailablePort(cfg.preferredPort)
        if port == 0 {
            return fmt.Errorf("no available ports found for %s (tried %d-%d)", cfg.name, cfg.preferredPort, cfg.preferredPort+99)
        }
        actual[svc] = port
        mapping := fmt.Sprintf("%d:%d", port, cfg.targetPort)
        args := []string{"port-forward", "-n", "glassflow", "service/" + svc, mapping}
        if kubeContext != "" {
            args = append([]string{"--context", kubeContext}, args...)
        }
        cmd := exec.Command("kubectl", args...)
        cmd.Stdout = os.Stdout
        cmd.Stderr = os.Stderr
        if err := cmd.Start(); err != nil {
            return fmt.Errorf("failed to start port forwarding for %s: %w", svc, err)
        }
        if cmd.Process != nil {
            st.Entries = append(st.Entries, portForwardEntry{PID: cmd.Process.Pid, Service: svc})
            _ = savePF(st)
        }
        time.Sleep(1 * time.Second)
    }

    fmt.Printf("🔗 Port forwarding established:\n")
    if p, ok := actual["glassflow-ui"]; ok {
        fmt.Printf("   🌐 GlassFlow UI: http://localhost:%d\n", p)
    }
    if p, ok := actual["glassflow-api"]; ok {
        fmt.Printf("   🔌 GlassFlow API: http://localhost:%d\n", p)
    }
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


