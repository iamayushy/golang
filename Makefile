.PHONY: run build test fmt vet

run:
	go run ./cmd/server

build:
	go build -o bin/server ./cmd/server

test:
	go test -race ./...

fmt:
	gofmt -w .

vet:
	go vet ./...
