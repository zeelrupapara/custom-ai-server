.PHONY: build test run fmt vet

build:
	go mod tidy
	go build -o bin/server ./cmd

test:
	go test ./pkg/... ./internal/handlers

run:
	@make build
	./bin/server

fmt:
	go fmt ./...

vet:
	go vet ./...
