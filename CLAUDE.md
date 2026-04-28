# cli

GlassFlow CLI (`glassflow`) — primary interface for local development, cluster management, and pipeline operations. Built with Cobra + Viper.

## Repo layout

```
main.go                 # Entry point
cmd/
  root.go               # Root command + global flags
  up.go                 # Start local cluster
  down.go               # Stop local cluster
  setup_demo.go         # Load demo data
  kube_context.go       # Kubernetes context helpers
  version.go            # Version command
internal/
  install/              # Kind cluster management
  helm/                 # Helm chart deployment
  k8s/                  # Kubernetes client + port-forwarding
  demo/                 # Demo setup (ClickHouse, Kafka producer, API)
  config/               # Configuration loading
  github/               # GitHub release fetching
  tracking/             # Usage telemetry
build/                  # Compiled binaries (gitignored)
```

## Commands

```bash
make build              # Build glassflow binary → build/glassflow
make build-all          # Multi-platform (linux/darwin/windows amd64+arm64)
make test               # Tests with coverage
make lint               # golangci-lint
make fmt                # go fmt
make run ARGS="up --demo"  # Run with arguments
```

## Key CLI commands

```bash
glassflow up            # Start local Kind cluster + all services
glassflow up --demo     # Start + load demo data (Kafka producer, sample pipelines)
glassflow setup-demo    # Load demo data into existing cluster
glassflow down          # Tear down local cluster
glassflow version       # Print version
```

## Key technology choices

| Purpose | Library |
|---------|---------|
| CLI framework | Cobra + Viper |
| Kubernetes client | client-go |
| Helm operations | helm SDK |
| Local cluster | Kind |

## Local dev workflow

The CLI orchestrates the full local stack. Running `glassflow up` will:
1. Create a Kind cluster
2. Deploy GlassFlow via Helm charts
3. Set up port-forwarding to API (`localhost:8081`) and other services

After `up`, the API is at `http://localhost:8081`.

## Boundaries with other repos

- Deploys `charts/` via embedded Helm operations
- Communicates with `clickhouse-etl` REST API
- Pulls operator images from `glassflow-etl-k8s-operator`

## Git & PR conventions

- Branch naming follows Linear ticket ID: `ETL-XYZ` or `username/ETL-XYZ-description`
- Reviewed by: Petr, Pablo, Kiran
- No `Co-Authored-By: Claude` or AI attribution in commits/PRs

## Domain context

For glossary, architecture diagrams, customer personas, and cross-repo workflows see the shared context repo (sibling directory):

```
../glassflow-agent-context/
  domain/glossary.md              # Key terms and definitions
  domain/deployment-topology.md   # How components fit together in prod
  projects/cli/                   # CLI-specific notes
  workflows/linear-tickets.md     # Ticket → branch → PR flow
```

Load these on demand when doing design work, writing PR descriptions, or when domain terminology is ambiguous. Don't load them for routine code tasks.
