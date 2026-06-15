.PHONY: build test vet lint

build:
	go build -o redoubt ./cmd/redoubt

test:
	go test ./...

vet:
	go vet ./...

lint: vet
	@if command -v staticcheck >/dev/null 2>&1; then staticcheck ./...; fi

clean:
	rm -f redoubt redoubt.exe
