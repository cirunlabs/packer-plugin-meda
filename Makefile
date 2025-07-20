# Variables
MEDA_VERSION ?= v0.2
PACKER_VERSION ?= 1.10.0
GO_VERSION ?= 1.21

.PHONY: help setup build-plugin install-plugin build-images build-image clean test lint validate-templates

# Default target
help: ## Show this help message
	@echo "Available commands:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-20s %s\n", $$1, $$2}'

# Setup everything from scratch
setup: ## Install all dependencies and setup environment
	@echo "🔧 Setting up development environment..."
	@echo "Installing system dependencies..."
	sudo apt-get update
	sudo apt-get install -y qemu-utils genisoimage bridge-utils iptables curl jq unzip
	@echo "Installing Packer..."
	@if ! command -v packer &> /dev/null; then \
		curl -fsSL https://releases.hashicorp.com/packer/$(PACKER_VERSION)/packer_$(PACKER_VERSION)_linux_amd64.zip -o /tmp/packer.zip && \
		unzip /tmp/packer.zip -d /tmp && \
		sudo mv /tmp/packer /usr/local/bin/ && \
		rm /tmp/packer.zip; \
	fi
	@echo "Installing Meda $(MEDA_VERSION)..."
	@if ! command -v meda &> /dev/null; then \
		curl -fsSL https://github.com/cirunlabs/meda/releases/download/$(MEDA_VERSION)/meda_Linux_x86_64.tar.gz -o /tmp/meda.tar.gz && \
		tar -xzf /tmp/meda.tar.gz -C /tmp && \
		sudo mv /tmp/meda /usr/local/bin/ && \
		chmod +x /usr/local/bin/meda && \
		rm /tmp/meda.tar.gz; \
	fi
	@echo "✅ Setup complete!"

# Build the Packer plugin
build-plugin: ## Build the Packer plugin binary
	@echo "🔨 Building Packer plugin..."
	cd plugin && go mod tidy && go build -o packer-plugin-meda
	@echo "✅ Plugin built successfully!"

# Install the plugin for Packer
install-plugin: build-plugin ## Install the Packer plugin
	@echo "📦 Installing Packer plugin..."
	cd plugin && packer plugins install --path packer-plugin-meda github.com/cirunlabs/meda
	@echo "✅ Plugin installed successfully!"

# Validate all Packer templates
validate-templates: install-plugin ## Validate all Packer templates
	@echo "🔍 Validating Packer templates..."
	@for dir in images/*/; do \
		if [ -f "$$dir/template.pkr.hcl" ]; then \
			echo "Validating $$dir..."; \
			cd "$$dir" && packer validate template.pkr.hcl && cd ../..; \
		fi \
	done
	@echo "✅ All templates validated!"

# Build all images
build-images: install-plugin start-meda ## Build all VM images
	@echo "🏗️ Building all images..."
	@for dir in images/*/; do \
		if [ -f "$$dir/template.pkr.hcl" ]; then \
			echo "Building image in $$dir..."; \
			cd "$$dir" && packer build template.pkr.hcl && cd ../..; \
		fi \
	done
	@echo "✅ All images built successfully!"

# Build a specific image
build-image: install-plugin start-meda ## Build specific image (usage: make build-image IMAGE=ubuntu-docker)
ifndef IMAGE
	@echo "❌ Please specify IMAGE variable. Example: make build-image IMAGE=ubuntu-docker"
	@echo "Available images:"
	@ls -1 images/
	@exit 1
endif
	@if [ ! -d "images/$(IMAGE)" ]; then \
		echo "❌ Image directory images/$(IMAGE) not found"; \
		exit 1; \
	fi
	@echo "🏗️ Building image: $(IMAGE)..."
	cd images/$(IMAGE) && packer build template.pkr.hcl
	@echo "✅ Image $(IMAGE) built successfully!"

# Start Meda API server in background
start-meda: ## Start Meda API server
	@echo "🚀 Starting Meda API server..."
	@if ! pgrep -f "meda serve" > /dev/null; then \
		echo "📝 Meda logs will be written to /tmp/meda-server.log"; \
		meda serve --host 127.0.0.1 --port 7777 > /tmp/meda-server.log 2>&1 & \
		sleep 5; \
		if curl -sf http://127.0.0.1:7777/api/v1/health > /dev/null; then \
			echo "✅ Meda API server started successfully (PID: $$(pgrep -f 'meda serve'))"; \
			echo "💡 View logs with: tail -f /tmp/meda-server.log"; \
		else \
			echo "❌ Failed to start Meda API server"; \
			echo "📋 Server logs:"; \
			cat /tmp/meda-server.log 2>/dev/null || echo "No logs found"; \
			exit 1; \
		fi \
	else \
		echo "✅ Meda API server already running (PID: $$(pgrep -f 'meda serve'))"; \
		echo "💡 View logs with: tail -f /tmp/meda-server.log"; \
	fi

# Stop Meda API server
stop-meda: ## Stop Meda API server
	@echo "🛑 Stopping Meda API server..."
	@if pgrep -f "meda serve" > /dev/null 2>&1; then \
		pkill -f "meda serve" > /dev/null 2>&1; \
		sleep 1; \
		echo "✅ Meda API server stopped"; \
	else \
		echo "ℹ️  Meda API server not running"; \
	fi

# Run tests
test: ## Run Go tests for the plugin
	@echo "🧪 Running tests..."
	cd plugin && go test -v ./...
	@echo "✅ Tests completed!"

# Run linting
lint: ## Run linting on the plugin code
	@echo "🔍 Running linter..."
	cd plugin && golangci-lint run
	@echo "✅ Linting completed!"

# Clean up build artifacts and stop services
clean: ## Clean up build artifacts and stop services
	@echo "🧹 Cleaning up..."
	@$(MAKE) stop-meda || true
	@rm -f plugin/packer-plugin-meda
	@rm -f /tmp/meda-server.log
	@echo "Cleaning up any leftover VMs and images..."
	@if command -v meda &> /dev/null; then \
		meda list --json 2>/dev/null | jq -r '.[].name' | grep -E '^packer-' | xargs -r -I {} meda delete {} 2>/dev/null || true; \
		meda images list --json 2>/dev/null | jq -r '.[].name' | grep -E 'packer-|temp-' | xargs -r -I {} meda images rm {} 2>/dev/null || true; \
	fi
	@echo "✅ Cleanup completed!"

# Quick development cycle
dev: clean check-whitespace build-plugin install-plugin validate-templates ## Quick development cycle: clean, check whitespace, build, install, validate

# Full build pipeline
all: setup dev ## Complete build pipeline: setup, build plugin, validate templates (images require base images)

# List available images
list-images: ## List available image templates
	@echo "📋 Available image templates:"
	@for dir in images/*/; do \
		if [ -f "$$dir/template.pkr.hcl" ]; then \
			image_name=$$(basename "$$dir"); \
			if [ -f "$$dir/metadata.json" ]; then \
				description=$$(jq -r '.description // "No description"' "$$dir/metadata.json"); \
				echo "  $$image_name - $$description"; \
			else \
				echo "  $$image_name"; \
			fi \
		fi \
	done

# View Meda server logs
logs: ## View Meda server logs (real-time)
	@echo "📋 Viewing Meda server logs..."
	@if [ -f /tmp/meda-server.log ]; then \
		tail -f /tmp/meda-server.log; \
	else \
		echo "❌ No Meda logs found. Start Meda with 'make start-meda' first"; \
	fi

# Test image push to registry
test-push: ## Test pushing an image to registry (usage: make test-push IMAGE=image-name REGISTRY=ghcr.io)
ifndef IMAGE
	@echo "❌ Please specify IMAGE variable. Example: make test-push IMAGE=my-image"
	@exit 1
endif
	@echo "🧪 Testing image push..."
	@if ! meda images list --json | jq -r '.[].name' | grep -q "$(IMAGE)"; then \
		echo "❌ Image '$(IMAGE)' not found locally"; \
		echo "Available images:"; \
		meda images list --json | jq -r '.[].name' || echo "No images found"; \
		exit 1; \
	fi
	@echo "🚀 Test pushing $(IMAGE) to registry..."
	@REGISTRY=${REGISTRY:-ghcr.io}; \
	TARGET_IMAGE="$$REGISTRY/cirunlabs/$(IMAGE):test"; \
	echo "📝 Target: $$TARGET_IMAGE"; \
	meda push "$(IMAGE)" "$$TARGET_IMAGE" --registry "$$REGISTRY" --dry-run

# Check for trailing whitespace in source files
check-whitespace: ## Check for trailing whitespace in source files
	@echo "🔍 Checking for trailing whitespace..."
	@files_with_whitespace=$$(find . -name "*.yml" -o -name "*.yaml" -o -name "*.md" -o -name "*.hcl" -o -name "Makefile" -o -name "*.go" | xargs grep -l " $$" 2>/dev/null || true); \
	if [ -n "$$files_with_whitespace" ]; then \
		echo "❌ Files with trailing whitespace found:"; \
		echo "$$files_with_whitespace"; \
		exit 1; \
	else \
		echo "✅ No trailing whitespace found"; \
	fi

# Fix trailing whitespace in source files
fix-whitespace: ## Remove trailing whitespace from source files
	@echo "🔧 Removing trailing whitespace..."
	@find . -name "*.yml" -o -name "*.yaml" -o -name "*.md" -o -name "*.hcl" -o -name "Makefile" -o -name "*.go" | xargs sed -i 's/[ \t]*$$//'
	@echo "✅ Trailing whitespace removed"

# Show status of services and tools
status: ## Show status of required tools and services
	@echo "🔍 System Status:"
	@echo -n "Packer: "; command -v packer >/dev/null && packer version || echo "❌ Not installed"
	@echo -n "Meda: "; command -v meda >/dev/null && meda --version || echo "❌ Not installed"
	@echo -n "Go: "; command -v go >/dev/null && go version || echo "❌ Not installed"
	@echo -n "Meda API: "; curl -sf http://127.0.0.1:7777/api/v1/health >/dev/null && echo "✅ Running" || echo "❌ Not running"
	@echo -n "Plugin: "; [ -f "plugin/packer-plugin-meda" ] && echo "✅ Built" || echo "❌ Not built"