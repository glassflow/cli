#!/usr/bin/env bash
# build.sh — Creates a tar archive of all container images needed by the
# GlassFlow local dev stack. The CLI loads this archive into Kind via
# `kind load image-archive` after cluster creation, avoiding individual pulls.
#
# Usage: ./build.sh [--output PATH] [--push-oci REGISTRY/REPO:TAG]
#
# The build process:
#   1. Resolves all required images via resolve-images.sh
#   2. Pulls all images locally (linux/amd64)
#   3. Saves them as a single tar archive
#   4. Optionally packages as an OCI artifact and pushes to a registry
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# Defaults
OUTPUT_DIR="${SCRIPT_DIR}/output"
OUTPUT_NAME="glassflow-images"
PUSH_OCI=""

# Parse arguments
while [[ $# -gt 0 ]]; do
  case "$1" in
    --output) OUTPUT_DIR="$2"; shift 2 ;;
    --push-oci) PUSH_OCI="$2"; shift 2 ;;
    *) echo "Unknown option: $1" >&2; exit 1 ;;
  esac
done

mkdir -p "$OUTPUT_DIR"
OUTPUT_TAR="${OUTPUT_DIR}/${OUTPUT_NAME}.tar"

echo "============================================"
echo "Building GlassFlow image bundle"
echo "  Output: ${OUTPUT_TAR}"
echo "============================================"
echo ""

# Step 1: Resolve all required images
echo "Step 1/3: Resolving container images..."
IMAGES=$("${SCRIPT_DIR}/resolve-images.sh")
IMAGE_COUNT=$(echo "$IMAGES" | wc -l | tr -d ' ')
echo "  Found ${IMAGE_COUNT} images to bundle"
echo ""

# Step 2: Pull all images locally (linux/amd64 to match Kind's containerd)
echo "Step 2/3: Pulling images (linux/amd64)..."
PULLED_IMAGES=()
PULL_FAILURES=0
while IFS= read -r img; do
  echo "  Pulling: ${img}"
  if docker pull --platform linux/amd64 "$img" --quiet >/dev/null 2>&1; then
    PULLED_IMAGES+=("$img")
  else
    echo "  WARNING: Failed to pull ${img}, skipping"
    PULL_FAILURES=$((PULL_FAILURES + 1))
  fi
done <<< "$IMAGES"
if [ "$PULL_FAILURES" -gt 0 ]; then
  echo "  WARNING: ${PULL_FAILURES} image(s) failed to pull"
fi
echo "  Successfully pulled ${#PULLED_IMAGES[@]} images"
echo ""

# Step 3: Save all images to a single tar archive
echo "Step 3/3: Saving images to ${OUTPUT_TAR}..."
docker save "${PULLED_IMAGES[@]}" -o "$OUTPUT_TAR"

# Compress
echo "  Compressing..."
gzip -f "$OUTPUT_TAR"
OUTPUT_TAR="${OUTPUT_TAR}.gz"

SIZE=$(ls -lh "$OUTPUT_TAR" | awk '{print $5}')
echo ""
echo "============================================"
echo "Build complete!"
echo "  Archive: ${OUTPUT_TAR}"
echo "  Size:    ${SIZE}"
echo "  Images:  ${#PULLED_IMAGES[@]}"
echo ""
echo "Load into a Kind cluster with:"
echo "  kind load image-archive ${OUTPUT_TAR} --name glassflow"
echo "============================================"

# Push as OCI artifact if requested
if [ -n "$PUSH_OCI" ]; then
  echo ""
  echo "Pushing OCI artifact: ${PUSH_OCI}..."
  # Use ORAS to push the tar as an OCI artifact
  if command -v oras >/dev/null 2>&1; then
    oras push "$PUSH_OCI" "$OUTPUT_TAR:application/vnd.docker.image.rootfs.diff.tar.gzip"
    echo "  Push complete"
  else
    echo "  ERROR: oras CLI not found. Install from https://oras.land/"
    echo "  Alternatively, upload the tar manually to your artifact store."
    exit 1
  fi
fi
