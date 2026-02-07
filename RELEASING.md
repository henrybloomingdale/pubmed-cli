# Release Workflow

Follow this checklist for every release of pubmed-cli.

## Pre-Release Checklist

### 1. Code Quality
- [ ] All tests pass: `go test ./...`
- [ ] No race conditions: `go test -race ./...`
- [ ] Lint clean: `golangci-lint run ./...` (if available)
- [ ] Build succeeds: `go build ./cmd/pubmed`

### 2. Documentation Updates
- [ ] **README.md** is current:
  - [ ] Feature list matches actual commands
  - [ ] All flags documented
  - [ ] Examples work as shown
  - [ ] Architecture section reflects current package structure
  - [ ] Version badge updated
- [ ] **CHANGELOG.md** has entry for new version:
  - [ ] Added section with new features
  - [ ] Changed section with modifications  
  - [ ] Fixed section with bug fixes
  - [ ] Date is correct
- [ ] **Code comments** explain non-obvious logic
- [ ] **--help text** is clear and has examples

### 3. Version Bump
- [ ] Update version in `Makefile` (VERSION variable)
- [ ] Update version badge in `README.md`

## Release Steps

### 1. Commit All Changes
```bash
git add -A
git status  # Review changes
git commit -m "prepare release vX.Y.Z"
git push origin main
```

### 2. Create Git Tag
```bash
git tag -a vX.Y.Z -m "Brief description of release"
git push origin vX.Y.Z
```

### 3. Build Release Binaries
```bash
make release
# Creates: pubmed-darwin-arm64, pubmed-darwin-amd64, pubmed-linux-amd64
```

### 4. Create GitHub Release
```bash
gh release create vX.Y.Z \
  pubmed-darwin-arm64 \
  pubmed-darwin-amd64 \
  pubmed-linux-amd64 \
  --title "vX.Y.Z - Release Title" \
  --notes "## What's New

### Features
- Feature 1
- Feature 2

### Fixes
- Fix 1

See [CHANGELOG.md](https://github.com/henrybloomingdale/pubmed-cli/blob/main/CHANGELOG.md) for details."
```

### 5. Update Homebrew Formula

Get SHA256 hashes for the uploaded binaries:
```bash
curl -sL https://github.com/henrybloomingdale/pubmed-cli/releases/download/vX.Y.Z/pubmed-darwin-arm64 | shasum -a 256
curl -sL https://github.com/henrybloomingdale/pubmed-cli/releases/download/vX.Y.Z/pubmed-darwin-amd64 | shasum -a 256
```

Update `~/github/homebrew-tools/Formula/pubmed-cli.rb`:
```ruby
class PubmedCli < Formula
  # ...
  version "X.Y.Z"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/henrybloomingdale/pubmed-cli/releases/download/vX.Y.Z/pubmed-darwin-arm64"
      sha256 "<arm64-sha256>"
    else
      url "https://github.com/henrybloomingdale/pubmed-cli/releases/download/vX.Y.Z/pubmed-darwin-amd64"
      sha256 "<amd64-sha256>"
    end
  end
  # ...
end
```

Commit and push:
```bash
cd ~/github/homebrew-tools
git add Formula/pubmed-cli.rb
git commit -m "pubmed-cli X.Y.Z: brief description"
git push
```

### 6. Verify Installation
```bash
brew update
brew upgrade pubmed-cli
pubmed --help
pubmed qa --help  # If qa command exists
```

## Quick Release Script

For convenience, here's the full flow:

```bash
#!/bin/bash
set -e

VERSION=$1
if [ -z "$VERSION" ]; then
  echo "Usage: ./release.sh X.Y.Z"
  exit 1
fi

echo "=== Building v$VERSION ==="
go test ./...
make release

echo "=== Tagging v$VERSION ==="
git tag -a "v$VERSION" -m "Release v$VERSION"
git push origin "v$VERSION"

echo "=== Creating GitHub Release ==="
gh release create "v$VERSION" \
  pubmed-darwin-arm64 \
  pubmed-darwin-amd64 \
  pubmed-linux-amd64 \
  --title "v$VERSION" \
  --notes "See CHANGELOG.md for details."

echo "=== Getting SHA256 hashes ==="
ARM_SHA=$(curl -sL "https://github.com/henrybloomingdale/pubmed-cli/releases/download/v$VERSION/pubmed-darwin-arm64" | shasum -a 256 | cut -d' ' -f1)
AMD_SHA=$(curl -sL "https://github.com/henrybloomingdale/pubmed-cli/releases/download/v$VERSION/pubmed-darwin-amd64" | shasum -a 256 | cut -d' ' -f1)

echo ""
echo "=== Update Homebrew Formula ==="
echo "ARM64 SHA256: $ARM_SHA"
echo "AMD64 SHA256: $AMD_SHA"
echo ""
echo "Edit: ~/github/homebrew-tools/Formula/pubmed-cli.rb"
echo "Then: cd ~/github/homebrew-tools && git add -A && git commit -m 'pubmed-cli $VERSION' && git push"
```

## Post-Release

- [ ] Run `brew upgrade pubmed-cli` to verify
- [ ] Test a few commands to ensure binary works
- [ ] Announce if significant (Discord, etc.)
