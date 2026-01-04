#!/bin/bash
# Release helper script for Omnia
#
# Usage: ./scripts/release.sh <version>
# Example: ./scripts/release.sh 0.2.0
#          ./scripts/release.sh 0.3.0-beta.1

set -e

VERSION=$1

if [ -z "$VERSION" ]; then
  echo "Usage: ./scripts/release.sh <version>"
  echo ""
  echo "Examples:"
  echo "  ./scripts/release.sh 0.2.0        # Stable release"
  echo "  ./scripts/release.sh 0.3.0-beta.1 # Pre-release"
  echo ""
  echo "The version should NOT include the 'v' prefix."
  exit 1
fi

# Validate semver format
if ! [[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9]+(\.[0-9]+)?)?$ ]]; then
  echo "Error: Invalid semantic version: $VERSION"
  echo "Expected format: MAJOR.MINOR.PATCH or MAJOR.MINOR.PATCH-prerelease"
  exit 1
fi

# Ensure we're on main branch
CURRENT_BRANCH=$(git branch --show-current)
if [ "$CURRENT_BRANCH" != "main" ]; then
  echo "Warning: You are not on the main branch (currently on: $CURRENT_BRANCH)"
  read -p "Continue anyway? (y/N) " -n 1 -r
  echo
  if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    exit 1
  fi
fi

# Check for uncommitted changes
if ! git diff --quiet HEAD; then
  echo "Error: You have uncommitted changes. Please commit or stash them first."
  git status --short
  exit 1
fi

# Pull latest changes
echo "Pulling latest changes from origin..."
git pull origin "$CURRENT_BRANCH"

# Run local validation
echo ""
echo "Running local validation..."
echo ""

echo "==> Linting Helm chart..."
helm lint charts/omnia

echo ""
echo "==> Building Go binaries..."
make build

echo ""
echo "==> Running tests..."
make test

# Confirm release
echo ""
echo "=========================================="
echo "Ready to create release v$VERSION"
echo "=========================================="
echo ""
echo "This will:"
echo "  1. Create and push git tag v$VERSION"
echo "  2. Trigger GitHub Actions to:"
echo "     - Build and push Docker images to GHCR"
echo "     - Package and push Helm chart to GHCR"
echo "     - Build and deploy documentation"
echo "     - Create GitHub Release with artifacts"
echo ""

read -p "Create release v$VERSION? (y/N) " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
  echo "Release cancelled."
  exit 1
fi

# Create and push tag
echo ""
echo "Creating tag v$VERSION..."
git tag -a "v$VERSION" -m "Release v$VERSION"

echo "Pushing tag to origin..."
git push origin "v$VERSION"

echo ""
echo "Release v$VERSION triggered!"
echo ""
echo "Monitor the release progress at:"
echo "  https://github.com/AltairaLabs/omnia/actions"
echo ""
echo "Once complete, the release will be available at:"
echo "  https://github.com/AltairaLabs/omnia/releases/tag/v$VERSION"
