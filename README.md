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

**Step 1: Start GlassFlow**

```bash
glassflow up
```

This creates a Kind cluster and installs GlassFlow. Once ready, you can access:
- **GlassFlow UI**: http://localhost:30080
- **GlassFlow API**: http://localhost:30180

From the UI, you can connect to your own Kafka and ClickHouse instances.

**Step 2 (optional): Run the demo**

To see data flowing end-to-end with a local Kafka and ClickHouse:

```bash
glassflow setup-demo
```

This installs Kafka + ClickHouse, creates a demo pipeline, and starts a Kafka producer sending sample events.

**All-in-one** (GlassFlow + Kafka + ClickHouse + demo pipeline):

```bash
glassflow up --demo
```

**Stop the environment:**

```bash
glassflow down
```

## What Gets Installed

**`glassflow up`** creates a Kind cluster and installs:
- **GlassFlow ETL** (API, UI, operator, NATS, PostgreSQL) in the `glassflow` namespace
- Port forwarding for the GlassFlow UI and API

**`glassflow setup-demo`** adds:
- **Kafka** (kafka namespace) and **ClickHouse** (clickhouse namespace)
- A demo pipeline with a Kafka producer sending sample events
- Port forwarding for ClickHouse HTTP (http://localhost:30090)

**`glassflow up --demo`** does both in one step.

## Commands

```bash
# Start GlassFlow (Kind cluster + GlassFlow only)
glassflow up

# Set up demo (installs Kafka + ClickHouse + demo pipeline)
glassflow setup-demo

# All-in-one (GlassFlow + Kafka + ClickHouse + demo)
glassflow up --demo

# Stop and clean up
glassflow down

# Force stop (skip Helm uninstall, delete cluster directly)
glassflow down --force

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

