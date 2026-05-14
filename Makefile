.PHONY: build test clean release release-all upx \
	release-linux-amd64 release-linux-arm64 \
	release-darwin-arm64 release-darwin-amd64

BINARY  := brun
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION)"
SRC     := .
BINDIR  := bin

TARGETS := linux/amd64 linux/arm64 darwin/arm64 darwin/amd64

## ── 开发 ─────────────────────────────────────────────

build:
	@mkdir -p $(BINDIR)
	CGO_ENABLED=0 go build -o $(BINDIR)/$(BINARY) $(LDFLAGS) $(SRC)

test:
	go test ./... -v -count=1

test-fast:
	go test ./... -count=1

clean:
	rm -rf $(BINDIR)/
	rm -rf dist/

## ── 打包（当前平台，upx 压缩）──────────────────────────

release: build
	upx --best --lzma $(BINDIR)/$(BINARY)
	@echo "=== Release: $(BINDIR)/$(BINARY) ==="
	@ls -lh $(BINDIR)/$(BINARY)

## ── 交叉编译 ─────────────────────────────────────────

release-all: $(TARGETS)
	@for t in $(TARGETS); do \
		os=$${t%/*}; arch=$${t#*/}; \
		dst="dist/$(BINARY)-$$os-$$arch"; \
		if [ "$$os" = "linux" ]; then \
			upx --best --lzma "$$dst" 2>/dev/null || true; \
		fi; \
	done
	@echo "=== All releases ==="
	@ls -lh dist/

dist/$(BINARY)-%-amd64:
	CGO_ENABLED=0 GOOS=$* GOARCH=amd64 go build -o $@ $(LDFLAGS) $(SRC)

dist/$(BINARY)-%-arm64:
	CGO_ENABLED=0 GOOS=$* GOARCH=arm64 go build -o $@ $(LDFLAGS) $(SRC)

release-linux-amd64: dist/$(BINARY)-linux-amd64
	upx --best --lzma $<

release-linux-arm64: dist/$(BINARY)-linux-arm64
	upx --best --lzma $<

release-darwin-arm64: dist/$(BINARY)-darwin-arm64
	@echo "macOS arm64 ready: $<"

release-darwin-amd64: dist/$(BINARY)-darwin-amd64
	@echo "macOS amd64 ready: $<"
