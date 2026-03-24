#!/usr/bin/env bash
# resolve-images.sh — Resolves all container images required by the GlassFlow
# local development stack (GlassFlow ETL + Kafka + ClickHouse).
#
# Usage: ./resolve-images.sh [--values-file PATH]
#
# Runs `helm template` for each chart with the exact values the CLI uses,
# parses all image: references, and outputs a deduplicated list.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
CLI_DIR="$(dirname "$SCRIPT_DIR")"
CHART_DIR="$(dirname "$CLI_DIR")/charts/charts/glassflow-etl"

# Defaults from cli/internal/config/default_config.yaml
GLASSFLOW_VALUES="${SCRIPT_DIR}/../internal/install/glassflow_values.yaml"
KAFKA_VERSION="32.4.3"
CLICKHOUSE_VERSION="9.4.4"

# Ensure helm repos are available
helm repo add glassflow https://glassflow.github.io/charts >/dev/null 2>&1 || true
helm repo add bitnami https://charts.bitnami.com/bitnami >/dev/null 2>&1 || true
helm repo update >/dev/null 2>&1

TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

# 1. GlassFlow ETL chart (includes NATS, PostgreSQL as sub-chart dependencies)
echo "Resolving GlassFlow ETL images..." >&2
helm template glassflow glassflow/glassflow-etl \
  -f "$GLASSFLOW_VALUES" \
  --namespace glassflow \
  2>/dev/null > "$TMPDIR/glassflow.yaml" || true

# 2. Kafka chart (Bitnami)
echo "Resolving Kafka images..." >&2
helm template kafka bitnami/kafka \
  --version "$KAFKA_VERSION" \
  --set image.registry=docker.io \
  --set image.repository=bitnamilegacy/kafka \
  --set controller.replicaCount=1 \
  --namespace kafka \
  2>/dev/null > "$TMPDIR/kafka.yaml" || true

# 3. ClickHouse chart (Bitnami)
echo "Resolving ClickHouse images..." >&2
helm template clickhouse bitnami/clickhouse \
  --version "$CLICKHOUSE_VERSION" \
  --set image.registry=docker.io \
  --set image.repository=bitnamilegacy/clickhouse \
  --set keeper.image.registry=docker.io \
  --set keeper.image.repository=bitnamilegacy/clickhouse-keeper \
  --namespace clickhouse \
  2>/dev/null > "$TMPDIR/clickhouse.yaml" || true

# Extract all image references from rendered manifests
# Handles both `image: foo` and `- image: foo` patterns
grep -hE '^\s+image:\s' "$TMPDIR"/*.yaml \
  | sed 's/.*image:\s*//' \
  | tr -d '"' \
  | tr -d "'" \
  | sed 's/^[[:space:]]*//;s/[[:space:]]*$//' \
  | grep -v '^$' \
  | sort -u > "$TMPDIR/images.txt"

# Add images not in Helm charts but used by the CLI
# Demo producer (created by cli/internal/demo/producer.go)
echo "python:3.11-slim" >> "$TMPDIR/images.txt"

# Deduplicate and output
sort -u "$TMPDIR/images.txt"
