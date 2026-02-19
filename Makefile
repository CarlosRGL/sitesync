VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BINARY   := sitesync
BUILD    := go build -ldflags "-s -w -X main.version=$(VERSION)"
DIST     := dist

.PHONY: build install clean release all

# ── Default: build for current OS/arch ───────────────────
build:
	$(BUILD) -o $(BINARY) ./cmd/sitesync

install: build
	cp $(BINARY) ~/bin/$(BINARY)
	@echo "✔ installed to ~/bin/$(BINARY)"

# ── Cross-compile all platforms ──────────────────────────
release: clean
	@mkdir -p $(DIST)
	GOOS=darwin  GOARCH=amd64 $(BUILD) -o $(DIST)/$(BINARY)-darwin-amd64   ./cmd/sitesync
	GOOS=darwin  GOARCH=arm64 $(BUILD) -o $(DIST)/$(BINARY)-darwin-arm64   ./cmd/sitesync
	GOOS=linux   GOARCH=amd64 $(BUILD) -o $(DIST)/$(BINARY)-linux-amd64    ./cmd/sitesync
	GOOS=linux   GOARCH=arm64 $(BUILD) -o $(DIST)/$(BINARY)-linux-arm64    ./cmd/sitesync
	@echo "✔ binaries in $(DIST)/"
	@ls -lh $(DIST)/

clean:
	rm -rf $(DIST) $(BINARY)

all: release
