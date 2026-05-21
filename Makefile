.PHONY: all build test lint clean

all: lint test build

build:
	go build -o bin/modenv cmd/modenv/main.go

test:
	go test -v ./...

lint:
	golangci-lint run

clean:
	rm -rf bin/
