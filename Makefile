.PHONY: build run dev clean docker-build docker-up docker-down docker-rebuild docker-logs help

# App details
APP_NAME=infokeep
BIN_DIR=bin

# Default target
all: build

## 🏗️  Local Development
build: ## Build the Go application locally
	@echo "Building $(APP_NAME)..."
	go build -o $(BIN_DIR)/$(APP_NAME) .

run: ## Run the Go application directly
	@echo "Running $(APP_NAME)..."
	go run .

clean: ## Clean up built binaries
	@echo "Cleaning up..."
	go clean
	rm -rf $(BIN_DIR)

## 🐳 Docker Management
docker-build: ## Build the docker image
	@echo "Building docker image..."
	docker compose build

docker-up: ## Start the application in Docker (background)
	@echo "Starting docker containers..."
	docker compose up -d

docker-down: ## Stop and remove the Docker containers
	@echo "Stopping docker containers..."
	docker compose down

docker-rebuild: ## Completely rebuild and restart Docker containers
	@echo "Rebuilding and restarting docker containers..."
	docker compose down
	docker compose build --no-cache
	docker compose up -d

docker-logs: ## Tail the Docker logs
	docker compose logs -f

## 🛠️  Utils
help: ## Show this help message
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)
