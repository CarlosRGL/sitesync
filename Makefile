VERSION      ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BINARY       := sitesync
BUILD        := go build -ldflags "-s -w -X main.version=$(VERSION)"
DIST         := dist
GITLAB_URL   ?= https://gitlab.quai13.net
PROJECT_PATH := teamtreize%2Fsitesync

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

# ── Upload binaries to GitLab release ────────────────────
# Requires: GITLAB_TOKEN env var (api scope)
# Usage:    GITLAB_TOKEN=xxx make publish
publish:
	@test -n "$(GITLAB_TOKEN)" || { echo "error: GITLAB_TOKEN is not set"; exit 1; }
	@echo "Publishing $(VERSION) to GitLab..."
	@echo "  ensuring release exists..."
	@curl -sf \
	  --header "PRIVATE-TOKEN: $(GITLAB_TOKEN)" \
	  --header "Content-Type: application/json" \
	  --request POST \
	  --data "{\"tag_name\":\"$(VERSION)\",\"name\":\"$(VERSION)\",\"ref\":\"$(shell git rev-parse HEAD)\"}" \
	  "$(GITLAB_URL)/api/v4/projects/$(PROJECT_PATH)/releases" > /dev/null \
	|| echo "  release already exists, continuing..."
	@for p in darwin-amd64 darwin-arm64 linux-amd64 linux-arm64; do \
	  echo "  uploading $(BINARY)-$$p..."; \
	  curl -sf \
	    --header "PRIVATE-TOKEN: $(GITLAB_TOKEN)" \
	    --upload-file "$(DIST)/$(BINARY)-$$p" \
	    "$(GITLAB_URL)/api/v4/projects/$(PROJECT_PATH)/packages/generic/$(BINARY)/$(VERSION)/$(BINARY)-$$p" \
	  || { echo "✘ upload failed: $(BINARY)-$$p"; exit 1; }; \
	  pkg_url="$(GITLAB_URL)/api/v4/projects/$(PROJECT_PATH)/packages/generic/$(BINARY)/$(VERSION)/$(BINARY)-$$p"; \
	  curl -sf \
	    --header "PRIVATE-TOKEN: $(GITLAB_TOKEN)" \
	    --header "Content-Type: application/json" \
	    --request POST \
	    --data "{\"name\":\"$(BINARY)-$$p\",\"url\":\"$$pkg_url\",\"link_type\":\"package\"}" \
	    "$(GITLAB_URL)/api/v4/projects/$(PROJECT_PATH)/releases/$(VERSION)/assets/links" > /dev/null \
	  || echo "  ⚠  link already exists for $$p (skipping)"; \
	done
	@echo "✔ Published $(VERSION)"

clean:
	rm -rf $(DIST) $(BINARY) coverage.out coverage.html

all: release
