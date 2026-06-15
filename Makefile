BINARY   := redoubt
MODULE   := github.com/kevinthelago/redoubt
VERSION  := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS  := -ldflags "-X $(MODULE)/internal/cli.Version=$(VERSION)"
BIN_DIR  := bin

.PHONY: all build test vet lint cross deps clean

all: vet test build

build:
	go build $(LDFLAGS) -o $(BIN_DIR)/$(BINARY) ./cmd/redoubt

test:
	go test -race ./...

vet:
	go vet ./...

lint:
	golangci-lint run ./...

cross:
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BIN_DIR)/$(BINARY)-windows-amd64.exe ./cmd/redoubt
	GOOS=linux   GOARCH=amd64 go build $(LDFLAGS) -o $(BIN_DIR)/$(BINARY)-linux-amd64        ./cmd/redoubt
	GOOS=linux   GOARCH=arm64 go build $(LDFLAGS) -o $(BIN_DIR)/$(BINARY)-linux-arm64         ./cmd/redoubt

deps:
	go mod tidy

clean:
	rm -rf $(BIN_DIR)/
