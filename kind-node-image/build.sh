#!/usr/bin/env bash
# build.sh — Creates tar archives of container images for the GlassFlow local dev stack.
#
# Produces two bundles:
#   glassflow-images.tar.gz  — GlassFlow, NATS, PostgreSQL, utilities (for `glassflow up`)
#   demo-images.tar.gz       — Kafka, ClickHouse, demo producer (for `glassflow setup-demo`)
#
# Usage: ./build.sh [--output DIR]
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
OUTPUT_DIR="${SCRIPT_DIR}/output"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --output) OUTPUT_DIR="$2"; shift 2 ;;
    *) echo "Unknown option: $1" >&2; exit 1 ;;
  esac
done

mkdir -p "$OUTPUT_DIR"

build_bundle() {
  local name="$1"
  local mode="$2"
  local output_tar="${OUTPUT_DIR}/${name}.tar"

  echo "--- Building ${name} ---"

  echo "  Resolving images..."
  local images
  images=$("${SCRIPT_DIR}/resolve-images.sh" "$mode")
  local count
  count=$(echo "$images" | wc -l | tr -d ' ')
  echo "  Found ${count} images"

  echo "  Pulling images (linux/amd64)..."
  local pulled=()
  while IFS= read -r img; do
    if docker pull --platform linux/amd64 "$img" --quiet >/dev/null 2>&1; then
      pulled+=("$img")
    else
      echo "  WARNING: Failed to pull ${img}, skipping"
    fi
  done <<< "$images"
  echo "  Pulled ${#pulled[@]} images"

  echo "  Saving to ${output_tar}..."
  docker save "${pulled[@]}" -o "$output_tar"

  echo "  Compressing..."
  gzip -f "$output_tar"

  local size
  size=$(ls -lh "${output_tar}.gz" | awk '{print $5}')
  echo "  Done: ${output_tar}.gz (${size}, ${#pulled[@]} images)"
  echo ""
}

echo "============================================"
echo "Building GlassFlow image bundles"
echo "============================================"
echo ""

build_bundle "glassflow-images" "--core"
build_bundle "demo-images" "--demo"

echo "============================================"
echo "Build complete!"
echo ""
echo "  Core:  ${OUTPUT_DIR}/glassflow-images.tar.gz"
echo "  Demo:  ${OUTPUT_DIR}/demo-images.tar.gz"
echo ""
echo "Place in ~/.glassflow/ for automatic loading:"
echo "  cp ${OUTPUT_DIR}/*.tar.gz ~/.glassflow/"
echo "============================================"
