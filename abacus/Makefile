.PHONY: build test clean install help

BUILD_DIR := .
INSTALL_DIR := $(HOME)/.local/bin
BINARY := abacus

build:
	@echo "Building $(BINARY)..."
	go build -ldflags="-X main.Build=$$(git rev-parse --short HEAD 2>/dev/null || echo unknown)" -o $(BUILD_DIR)/$(BINARY) .

test:
	@echo "Running tests..."
	go test ./...

clean:
	rm -f $(BINARY)

install: build
	@mkdir -p $(INSTALL_DIR)
	@cp $(BUILD_DIR)/$(BINARY) $(INSTALL_DIR)/$(BINARY)
	@echo "Installed $(BINARY) to $(INSTALL_DIR)/$(BINARY)"

help:
	@echo "abacus Makefile targets:"
	@echo "  make build   - Build the abacus binary"
	@echo "  make test    - Run tests"
	@echo "  make install - Build and install to ~/.local/bin"
	@echo "  make clean   - Remove build artifacts"
