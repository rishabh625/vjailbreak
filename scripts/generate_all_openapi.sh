#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$SCRIPT_DIR/.."

# Copy the latest script/template to /tmp
cp "$SCRIPT_DIR/generate_openapi.sh" /tmp/
cp -r "$PROJECT_ROOT/swagger-ui-template" /tmp/

ORIGINAL_BRANCH=$(git rev-parse --abbrev-ref HEAD)

# Persistent output directory
BUILD_OUTPUT="/tmp/swagger-ui-build"
rm -rf "$BUILD_OUTPUT"
mkdir -p "$BUILD_OUTPUT"

for TAG in $(git tag --sort=-creatordate | head -n 5); do
  git stash --include-untracked
  git checkout $TAG

  PROJECT_ROOT=$(git rev-parse --show-toplevel)
  CRD_DIR="$PROJECT_ROOT/k8s/migration/config/crd/bases"

  echo "Checking CRD directory: $CRD_DIR"
  ls -l "$CRD_DIR" 2>/dev/null || echo "Directory missing"

  if [ -d "$CRD_DIR" ] && compgen -G "$CRD_DIR/*.yaml" >/dev/null; then
    echo "$TAG: Generating OpenAPI..."
    chmod +x /tmp/generate_openapi.sh
    /tmp/generate_openapi.sh "$TAG" "$PROJECT_ROOT"

    OUTPUT_DIR="$BUILD_OUTPUT/$TAG"
    mkdir -p "$OUTPUT_DIR"
    cp -r /tmp/swagger-ui-template/* "$OUTPUT_DIR/"
    cp "$PROJECT_ROOT/docs/swagger-ui/$TAG/openapi.yaml" "$OUTPUT_DIR/openapi.yaml"
  else
    echo "$TAG: Skipping (no CRDs)"
  fi
done

# Switch back to the original branch
git checkout "$ORIGINAL_BRANCH"

# Now copy all generated folders into the real output directory
rm -rf "$PROJECT_ROOT/docs/public/swagger-ui"
mkdir -p "$PROJECT_ROOT/docs/public/swagger-ui"
cp -r /tmp/swagger-ui-build/* "$PROJECT_ROOT/docs/public/swagger-ui/"

echo "All Swagger UI versions copied to docs/public/swagger-ui/"
