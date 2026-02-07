#!/bin/bash
# Release script for pubmed-cli
# Usage: ./scripts/release.sh X.Y.Z

set -e

VERSION=$1
if [ -z "$VERSION" ]; then
  echo "Usage: ./scripts/release.sh X.Y.Z"
  exit 1
fi

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

echo "=== Pre-release checks ==="
go test ./...
echo "✓ Tests pass"

echo ""
echo "=== Building v$VERSION ==="
make release
echo "✓ Binaries built"

echo ""
echo "=== Tagging v$VERSION ==="
git tag -a "v$VERSION" -m "Release v$VERSION"
git push origin "v$VERSION"
echo "✓ Tag pushed"

echo ""
echo "=== Creating GitHub Release ==="
gh release create "v$VERSION" \
  pubmed-darwin-arm64 \
  pubmed-darwin-amd64 \
  pubmed-linux-amd64 \
  --title "v$VERSION" \
  --notes "See [CHANGELOG.md](https://github.com/henrybloomingdale/pubmed-cli/blob/main/CHANGELOG.md) for details."
echo "✓ GitHub release created"

echo ""
echo "=== Calculating SHA256 hashes ==="
sleep 2  # Wait for GitHub to process uploads

ARM_SHA=$(curl -sL "https://github.com/henrybloomingdale/pubmed-cli/releases/download/v$VERSION/pubmed-darwin-arm64" | shasum -a 256 | cut -d' ' -f1)
AMD_SHA=$(curl -sL "https://github.com/henrybloomingdale/pubmed-cli/releases/download/v$VERSION/pubmed-darwin-amd64" | shasum -a 256 | cut -d' ' -f1)

echo ""
echo "========================================"
echo "  UPDATE HOMEBREW FORMULA"
echo "========================================"
echo ""
echo "Version: $VERSION"
echo "ARM64 SHA256: $ARM_SHA"
echo "AMD64 SHA256: $AMD_SHA"
echo ""
echo "File: ~/github/homebrew-tools/Formula/pubmed-cli.rb"
echo ""

# Auto-update homebrew formula
FORMULA="$HOME/github/homebrew-tools/Formula/pubmed-cli.rb"
if [ -f "$FORMULA" ]; then
  echo "Updating formula automatically..."
  
  # Use sed to update version and SHA256 values
  sed -i '' "s/version \"[^\"]*\"/version \"$VERSION\"/" "$FORMULA"
  sed -i '' "s|releases/download/v[^/]*/pubmed-darwin-arm64|releases/download/v$VERSION/pubmed-darwin-arm64|" "$FORMULA"
  sed -i '' "s|releases/download/v[^/]*/pubmed-darwin-amd64|releases/download/v$VERSION/pubmed-darwin-amd64|" "$FORMULA"
  
  # Update SHA256 - this is trickier, need to match the pattern
  # ARM64 SHA is right after the arm64 URL
  # AMD64 SHA is right after the amd64 URL
  
  # Create temp file with updates
  awk -v arm_sha="$ARM_SHA" -v amd_sha="$AMD_SHA" '
    /pubmed-darwin-arm64/ { arm_next=1 }
    /pubmed-darwin-amd64/ { amd_next=1 }
    /sha256/ && arm_next { gsub(/sha256 "[^"]*"/, "sha256 \"" arm_sha "\""); arm_next=0 }
    /sha256/ && amd_next { gsub(/sha256 "[^"]*"/, "sha256 \"" amd_sha "\""); amd_next=0 }
    { print }
  ' "$FORMULA" > "$FORMULA.tmp" && mv "$FORMULA.tmp" "$FORMULA"
  
  echo "✓ Formula updated"
  echo ""
  echo "Review and push:"
  echo "  cd ~/github/homebrew-tools"
  echo "  git diff"
  echo "  git add -A && git commit -m 'pubmed-cli $VERSION' && git push"
else
  echo "Formula not found at $FORMULA"
  echo "Update manually with the SHA256 values above."
fi

echo ""
echo "========================================"
echo "  RELEASE COMPLETE"
echo "========================================"
echo ""
echo "Next steps:"
echo "  1. Push homebrew formula (see above)"
echo "  2. brew update && brew upgrade pubmed-cli"
echo "  3. Test: pubmed qa --help"
