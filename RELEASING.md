# Releasing the GlassFlow CLI

## When to release

Release a new CLI version whenever:
- The GlassFlow Helm chart is released with new image versions
- CLI code changes (bug fixes, new features)
- Kafka/ClickHouse chart versions are bumped in `internal/config/default_config.yaml`

## How to release

### 1. Verify chart versions

The image bundle is built from whatever the Helm charts currently publish. Check that the charts you want are live:

```bash
helm repo update
helm search repo glassflow/glassflow-etl --versions | head -5
```

### 2. Update default_config.yaml (if chart versions changed)

If Kafka or ClickHouse chart versions changed, update `internal/config/default_config.yaml`:

```yaml
charts:
  kafka:
    version: "32.4.3"    # ← update this
  clickhouse:
    version: "9.4.4"     # ← update this
```

GlassFlow chart version is not pinned (empty = latest from repo), so it picks up new releases automatically.

### 3. Verify the image bundle locally (optional)

```bash
cd kind-node-image
./resolve-images.sh    # check which images will be bundled
./build.sh             # build the bundle locally (~2 min)
```

### 4. Tag and push

```bash
git tag v0.x.x
git push origin v0.x.x
```

This triggers the GitHub Actions workflow which:
1. **GoReleaser** builds CLI binaries for all platforms and creates the GitHub release
2. **Image bundle** job runs `build.sh`, then attaches `glassflow-images.tar.gz` to the release

### 5. Verify the release

Check https://github.com/glassflow/cli/releases for:
- [ ] CLI binaries for linux/darwin/windows (amd64 + arm64)
- [ ] `glassflow-images.tar.gz` (~724 MB) — core GlassFlow images
- [ ] `demo-images.tar.gz` (~674 MB) — Kafka + ClickHouse images
- [ ] `checksums.txt`

### 6. Update Homebrew tap (manual)

If you maintain a Homebrew tap, update the formula with the new version and checksums.

## What's in the image bundles

Two separate bundles are built to match the modular install flow:

**`glassflow-images.tar.gz`** (loaded by `glassflow up`):
- GlassFlow ETL components (API, UI, operator, migration, ingestor, sink, join, dedup, notifier)
- NATS + config reloader
- PostgreSQL
- Utility images (curl, busybox, kubectl)

**`demo-images.tar.gz`** (loaded by `glassflow setup-demo` and `glassflow up --demo`):
- Kafka (Bitnami)
- ClickHouse + Keeper (Bitnami)
- Python (demo producer)

Image versions are resolved dynamically from the **published Helm charts** at build time — they are **not hardcoded** in the build script and are not affected by local development versions.

## How the bundles work

1. `resolve-images.sh` runs `helm template` on published charts → extracts image references
2. `build.sh` pulls images and saves them as two `.tar.gz` files (core + demo)
3. Users place files at `~/.glassflow/glassflow-images.tar.gz` and `~/.glassflow/demo-images.tar.gz`
4. `glassflow up` loads the core bundle; `setup-demo` and `--demo` also load the demo bundle
5. Result: pods start without pulling images (~24s load vs 10-15 min pulls)
