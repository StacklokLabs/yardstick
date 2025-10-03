# Yardstick Project Context

## Overview
Yardstick is a project for benchmarking and performance testing.

## Project Structure
- Go-based project with multiple packages
- Uses GitHub Actions for CI/CD
- Dependency management through Renovate

## Key Components
- Client components for server connections
- Process management services
- Performance benchmarking utilities

## Development Guidelines
- Main branch is the primary development branch
- Pull requests should target the main branch
- Dependencies are automatically updated via Renovate bot

## Recent Changes
- Fixed client link issues
- Renamed yardstick packages for better organization
- Updated GitHub Actions and pinned dependencies

## Build and Test
Standard Go project commands:
- `go build` - Build the project
- `go test ./...` - Run all tests
- `go mod tidy` - Clean up dependencies

## Contributing
- Create feature branches from main
- Submit pull requests for review
- Ensure tests pass before merging