.PHONY: run build test lint

run:
	go run cmd/server/main.go

build:
	go build -o bin/saruman cmd/server/main.go

test:
	go test ./... -v

lint:
	golangci-lint run ./...
