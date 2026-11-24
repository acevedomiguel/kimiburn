BIN := kimiburn
PKG := ./cmd/kimiburn
VERSION ?= dev
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build test fmt vet run tidy check

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) $(PKG)

test:
	go test ./...

fmt:
	gofmt -l -w .

vet:
	go vet ./...

run:
	go run $(PKG)

tidy:
	go mod tidy

check: fmt vet test
