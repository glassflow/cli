#!/usr/bin/env bash
# resolve-images.sh — Resolves container images required by GlassFlow local dev stack.
#
# Images are resolved from the PUBLISHED Helm charts (not local files) to ensure
# the bundle always matches what users will actually install.
#
# Usage:
#   ./resolve-images.sh              # all images
#   ./resolve-images.sh --core       # GlassFlow + NATS + PostgreSQL + operator components
#   ./resolve-images.sh --demo       # Kafka + ClickHouse + demo producer only
set -euo pipefail

KAFKA_VERSION="32.4.3"
CLICKHOUSE_VERSION="9.4.4"

MODE="${1:-all}"

# Ensure helm repos are available and up-to-date
helm repo add glassflow https://glassflow.github.io/charts >/dev/null 2>&1 || true
helm repo add bitnami https://charts.bitnami.com/bitnami >/dev/null 2>&1 || true
helm repo update >/dev/null 2>&1

TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

extract_images_from_yaml() {
  # Extract image: fields from rendered YAML manifests
  grep -hE '^\s+image:\s' "$@" 2>/dev/null \
    | sed 's/.*image:\s*//' \
    | tr -d '"' \
    | tr -d "'" \
    | sed 's/^[[:space:]]*//;s/[[:space:]]*$//' \
    | grep -v '^$'
}

resolve_core() {
  echo "Resolving GlassFlow ETL images from published chart..." >&2

  # Get the published chart's default values (not local files)
  VALUES=$(helm show values glassflow/glassflow-etl 2>/dev/null)

  # Extract global image registry
  REGISTRY=$(echo "$VALUES" | grep 'imageRegistry:' | head -1 | sed 's/.*imageRegistry:\s*//;s/"//g;s/[[:space:]]//g')

  # Render the chart using its own default values (no local overrides)
  helm template glassflow glassflow/glassflow-etl \
    --namespace glassflow \
    2>/dev/null > "$TMPDIR/glassflow.yaml" || true

  extract_images_from_yaml "$TMPDIR/glassflow.yaml"

  # The operator component images (ingestor, join, sink, dedup) are NOT rendered
  # by helm template because they are created dynamically by the operator when a
  # pipeline runs. Extract them from the chart values directly.
  echo "Resolving operator component images from chart values..." >&2
  for component in ingestor join sink dedup; do
    repo=$(echo "$VALUES" | grep -A3 "${component}:" | grep "repository:" | head -1 | sed 's/.*repository:\s*//;s/"//g;s/[[:space:]]//g')
    tag=$(echo "$VALUES" | grep -A3 "${component}:" | grep "tag:" | head -1 | sed 's/.*tag:\s*//;s/"//g;s/[[:space:]]//g')
    if [ -n "$repo" ] && [ -n "$tag" ]; then
      echo "${REGISTRY}${repo}:${tag}"
    fi
  done

  # Also include the notifier image
  notifier_repo=$(echo "$VALUES" | grep -A5 "notifier:" | grep "repository:" | head -1 | sed 's/.*repository:\s*//;s/"//g;s/[[:space:]]//g')
  notifier_tag=$(echo "$VALUES" | grep -A5 "notifier:" | grep "tag:" | head -1 | sed 's/.*tag:\s*//;s/"//g;s/[[:space:]]//g')
  if [ -n "$notifier_repo" ] && [ -n "$notifier_tag" ]; then
    echo "${REGISTRY}${notifier_repo}:${notifier_tag}"
  fi
}

resolve_demo() {
  # Kafka chart (Bitnami)
  echo "Resolving Kafka images from published chart..." >&2
  helm template kafka bitnami/kafka \
    --version "$KAFKA_VERSION" \
    --set image.registry=docker.io \
    --set image.repository=bitnamilegacy/kafka \
    --set controller.replicaCount=1 \
    --namespace kafka \
    2>/dev/null > "$TMPDIR/kafka.yaml" || true

  # ClickHouse chart (Bitnami)
  echo "Resolving ClickHouse images from published chart..." >&2
  helm template clickhouse bitnami/clickhouse \
    --version "$CLICKHOUSE_VERSION" \
    --set image.registry=docker.io \
    --set image.repository=bitnamilegacy/clickhouse \
    --set keeper.image.registry=docker.io \
    --set keeper.image.repository=bitnamilegacy/clickhouse-keeper \
    --namespace clickhouse \
    2>/dev/null > "$TMPDIR/clickhouse.yaml" || true

  extract_images_from_yaml "$TMPDIR/kafka.yaml" "$TMPDIR/clickhouse.yaml"

  # Demo producer image (created by CLI, not in any chart)
  echo "python:3.11-slim"
}

{
  case "$MODE" in
    --core)  resolve_core ;;
    --demo)  resolve_demo ;;
    all|*)   resolve_core; resolve_demo ;;
  esac
} | sort -u || true
