param (
    [string]$Target = "build"
)

$APP_NAME = "infokeep"
$BIN_DIR = "bin"

switch ($Target) {
    "build" {
        Write-Host "Building $APP_NAME..."
        if (!(Test-Path -Path $BIN_DIR)) {
            New-Item -ItemType Directory -Force -Path $BIN_DIR | Out-Null
        }
        go build -o "$BIN_DIR\$APP_NAME.exe" .
    }
    "run" {
        Write-Host "Running $APP_NAME..."
        go run .
    }
    "clean" {
        Write-Host "Cleaning up..."
        go clean
        if (Test-Path -Path $BIN_DIR) {
            Remove-Item -Recurse -Force $BIN_DIR
        }
    }
    "docker-build" {
        Write-Host "Building docker image..."
        docker compose build
    }
    "docker-up" {
        Write-Host "Starting docker containers..."
        docker compose up -d
    }
    "docker-down" {
        Write-Host "Stopping docker containers..."
        docker compose down
    }
    "docker-rebuild" {
        Write-Host "Rebuilding and restarting docker containers..."
        docker compose down
        docker compose build --no-cache
        docker compose up -d
    }
    "docker-logs" {
        docker compose logs -f
    }
    "help" {
        Write-Host "Usage: .\manage.ps1 [target]"
        Write-Host ""
        Write-Host "Targets:"
        Write-Host "  build             Build the Go application locally"
        Write-Host "  run               Run the Go application directly"
        Write-Host "  clean             Clean up built binaries"
        Write-Host "  docker-build      Build the docker image"
        Write-Host "  docker-up         Start the application in Docker (background)"
        Write-Host "  docker-down       Stop and remove the Docker containers"
        Write-Host "  docker-rebuild    Completely rebuild and restart Docker containers"
        Write-Host "  docker-logs       Tail the Docker logs"
    }
    default {
        Write-Host "Unknown target: $Target"
        Write-Host "Run '.\manage.ps1 help' for a list of valid targets."
    }
}
