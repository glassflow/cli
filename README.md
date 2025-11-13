<div align="center">
  <img src="https://docs.glassflow.dev/~gitbook/image?url=https%3A%2F%2F1082326815-files.gitbook.io%2F%7E%2Ffiles%2Fv0%2Fb%2Fgitbook-x-prod.appspot.com%2Fo%2Forganizations%252FaR82XtsD8fLEkzPmMtb7%252Fsites%252Fsite_8vNM9%252Flogo%252Fft4nLD8mKhRmqTJjDp3i%252Flogo-color.png%3Falt%3Dmedia%26token%3Deb19e3bf-195b-413f-9965-4c76112953a3&width=128&dpr=3&quality=100&sign=10efaa8d&sv=1" /><br /><br />
</div>
<p align="center">
<a href="https://join.slack.com/t/glassflowhub/shared_invite/zt-349m7lenp-IFeKSGfQwpJfIiQ7oyFFKg">
        <img src="https://img.shields.io/badge/slack-join-community?logo=slack&amp;logoColor=white&amp;style=flat"
            alt="Chat on Slack"></a>
<a href="https://github.com/glassflow/clickhouse-etl">
        <img src="https://img.shields.io/badge/GitHub-clickhouse--etl-blue?logo=github"
            alt="GlassFlow ETL"></a>

# GlassFlow CLI

**Local development environment for GlassFlow ETL**

The GlassFlow CLI provides a quick way to set up a local development environment for exploring and testing [GlassFlow](https://github.com/glassflow/clickhouse-etl) - an open-source ETL tool for real-time data processing from Kafka to ClickHouse.

> **Note**: This CLI is designed for **local testing, demos, and exploration only**. For production deployments, use the [official GlassFlow Helm charts](https://github.com/glassflow/charts).

## ⚡️ Quick Start

### Prerequisites

- **Docker** (or compatible runtime like Docker Desktop, OrbStack, Colima, or Podman)
- **kubectl** (installed automatically via Homebrew, or install manually)

### Installation

#### Install via Homebrew (Recommended)

```bash
brew tap glassflow/tap
brew install glassflow
```

#### Install from GitHub Releases

Download the latest release for your platform from [GitHub Releases](https://github.com/glassflow/cli/releases).

### Usage

Start the local development environment with demo data:

```bash
glassflow up --demo
```

This command will:
- Create a local Kubernetes cluster using [Kind](https://kind.sigs.k8s.io/)
- Install Kafka, ClickHouse, and GlassFlow using Helm charts
- Set up a demo pipeline with sample data
- Configure port forwarding for UI and API access

Once started, you can access:
- **GlassFlow UI**: http://localhost:30080
- **GlassFlow API**: http://localhost:30180
- **ClickHouse HTTP**: http://localhost:30090

Stop the environment:

```bash
glassflow down
```

## What Gets Installed

When running `glassflow up --demo`, the CLI installs:

- **Kind**: Local Kubernetes cluster
- **Kafka**: Message broker (Bitnami Helm chart)
- **ClickHouse**: Columnar database (Bitnami Helm chart)
- **GlassFlow ETL**: Real-time streaming ETL service (GlassFlow Helm chart)
- **Demo Pipeline**: Pre-configured pipeline with sample data

## Commands

```bash
# Start local environment with demo
glassflow up --demo

# Stop and clean up environment
glassflow down

# Show version
glassflow version

# Get help
glassflow --help
```

## Production Deployment

For production use, deploy GlassFlow using the official Helm charts:

- **Helm Charts Repository**: [github.com/glassflow/charts](https://github.com/glassflow/charts)
- **Installation Guide**: [docs.glassflow.dev/installation/kubernetes](https://docs.glassflow.dev/installation/kubernetes)

## Resources

- **GlassFlow ETL**: [github.com/glassflow/clickhouse-etl](https://github.com/glassflow/clickhouse-etl)
- **Documentation**: [docs.glassflow.dev](https://docs.glassflow.dev)
- **Slack Community**: [Join GlassFlow Hub](https://glassflowhub.slack.com/join/shared_invite/zt-349m7lenp-IFeKSGfQwpJfIiQ7oyFFKg#/shared-invite/email)
- **Helm Charts**: [github.com/glassflow/charts](https://github.com/glassflow/charts)

