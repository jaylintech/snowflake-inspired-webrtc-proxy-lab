.PHONY: all build build-all test test-race vet lint clean

BIN := bin

all: build test

build:
	go build -o $(BIN)/ ./cmd/...

build-all:
	go build -o $(BIN)/ ./cmd/...

test:
	go test -count=1 ./...

test-race:
	go test -race -count=1 ./...

test-verbose:
	go test -v -count=1 ./...

vet:
	go vet ./...

lint:
	golangci-lint run ./...

clean:
	rm -rf $(BIN)
