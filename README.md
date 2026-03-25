<div align="center">
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
- **Helm** (v3) – used to install charts (installed automatically via Homebrew, or [install manually](https://helm.sh/docs/intro/install/))
- **kubectl** (installed automatically via Homebrew, or install manually)

Give Docker enough resources (e.g. 6–8 GB RAM, 4 CPUs) so all pods can schedule. If pods stay **Pending**, increase memory/CPU in Docker Desktop → Settings → Resources.

### Installation

#### Install via Homebrew (Recommended)

```bash
brew tap glassflow/tap
brew install glassflow
```

#### Install from GitHub Releases

Download the latest release for your platform from [GitHub Releases](https://github.com/glassflow/cli/releases).

### Usage

**Recommended: two steps**

1. **Install** (create cluster and install services). This can take 10–20+ minutes on first run:

   ```bash
   glassflow up
   ```

   The CLI waits until GlassFlow, Kafka, and ClickHouse services are ready. If it seems stuck, it prints progress and hints (e.g. `kubectl get pods -n glassflow -n kafka -n clickhouse`).

2. **Set up the demo** (port-forwarding, ClickHouse table, pipeline, Kafka producer):

   ```bash
   glassflow setup-demo
   ```

**All-in-one** (install and demo in a single run):

```bash
glassflow up --demo
```

Once running, you can access (ports may vary if alternatives were chosen):
- **GlassFlow UI**: http://localhost:30080
- **GlassFlow API**: http://localhost:30180
- **ClickHouse HTTP**: http://localhost:30090

Stop the environment:

```bash
glassflow down
```

## What Gets Installed

When you run `glassflow up`, the CLI:

- Creates a **Kind** cluster (if needed)
- Installs **GlassFlow ETL** (glassflow namespace) via Helm
- Installs **Kafka** (kafka namespace) and **ClickHouse** (clickhouse namespace) via Helm
- Waits for all services to be ready (up to ~25 minutes)

Running `glassflow setup-demo` then:

- Starts port-forwarding for the UI, API, and ClickHouse
- Creates the ClickHouse demo table and the GlassFlow demo pipeline
- Deploys a Kafka producer that sends sample events to the pipeline

## Commands

```bash
# Install only (recommended first step)
glassflow up

# Set up demo pipeline (run after 'glassflow up' succeeds)
glassflow setup-demo

# Install and set up demo in one go
glassflow up --demo

# Stop and clean up environment
glassflow down

# Show version
glassflow version

# Use a custom config file
glassflow up -c /path/to/config.yaml
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

