VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BINARY      := sitesync
BUILD       := go build -ldflags "-s -w -X main.version=$(VERSION)"
DIST        := dist
GITHUB_REPO := CarlosRGL/sitesync

.PHONY: build install clean release publish all test test-race lint coverage fuzz

# ── Default: build for current OS/arch ───────────────────
build:
	$(BUILD) -o $(BINARY) ./cmd/sitesync

install: build
	cp $(BINARY) ~/bin/$(BINARY)
	@echo "✔ installed to ~/bin/$(BINARY)"

# ── Tests ─────────────────────────────────────────────────
test:
	go test ./...

test-race:
	go test -race ./...

coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "✔ coverage report: coverage.html"

lint:
	go vet ./...

fuzz:
	go test -fuzz=FuzzResilientReplaceLine -fuzztime=30s ./internal/sync/

# ── Cross-compile all platforms ──────────────────────────
release: clean
	@mkdir -p $(DIST)
	GOOS=darwin  GOARCH=amd64 $(BUILD) -o $(DIST)/$(BINARY)-darwin-amd64   ./cmd/sitesync
	GOOS=darwin  GOARCH=arm64 $(BUILD) -o $(DIST)/$(BINARY)-darwin-arm64   ./cmd/sitesync
	GOOS=linux   GOARCH=amd64 $(BUILD) -o $(DIST)/$(BINARY)-linux-amd64    ./cmd/sitesync
	GOOS=linux   GOARCH=arm64 $(BUILD) -o $(DIST)/$(BINARY)-linux-arm64    ./cmd/sitesync
	@echo "✔ binaries in $(DIST)/"
	@ls -lh $(DIST)/

# ── Upload binaries to GitHub release ────────────────────
# Requires: gh CLI authenticated (gh auth login)
# Usage:    make publish
publish:
	@command -v gh >/dev/null || { echo "error: gh CLI not installed"; exit 1; }
	@echo "Publishing $(VERSION) to GitHub..."
	@gh release create $(VERSION) \
	  --title "$(VERSION)" \
	  --notes "" \
	  --repo $(GITHUB_REPO) 2>/dev/null \
	|| echo "  release already exists, continuing..."
	@gh release upload $(VERSION) $(DIST)/$(BINARY)-* install.sh \
	  --repo $(GITHUB_REPO) \
	  --clobber
	@echo "✔ Published $(VERSION)"

clean:
	rm -rf $(DIST) $(BINARY) coverage.out coverage.html

all: release
