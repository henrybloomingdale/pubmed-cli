.PHONY: build test test-integration install lint clean release publish

VERSION := 0.3.0
BINARY := pubmed
PKG := ./cmd/pubmed

build:
	go build -o $(BINARY) $(PKG)

# Cross-compile for release
release:
	GOOS=darwin GOARCH=arm64 go build -o $(BINARY)-darwin-arm64 $(PKG)
	GOOS=darwin GOARCH=amd64 go build -o $(BINARY)-darwin-amd64 $(PKG)
	GOOS=linux GOARCH=amd64 go build -o $(BINARY)-linux-amd64 $(PKG)
	@echo "Built binaries for darwin/arm64, darwin/amd64, linux/amd64"

test:
	go test -short -count=1 ./...

test-integration:
	go test -tags integration -count=1 -v ./...

install:
	go install $(PKG)

lint:
	@which golangci-lint > /dev/null 2>&1 || echo "Install golangci-lint: https://golangci-lint.run/welcome/install/"
	golangci-lint run ./...

vet:
	go vet ./...

clean:
	rm -f $(BINARY)
	go clean

coverage:
	go test -short -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Full release: build, tag, upload, update homebrew
# Usage: make publish V=0.4.0
publish:
	@if [ -z "$(V)" ]; then echo "Usage: make publish V=X.Y.Z"; exit 1; fi
	./scripts/release.sh $(V)
