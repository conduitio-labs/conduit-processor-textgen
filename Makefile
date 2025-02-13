VERSION=$(shell git describe --tags --dirty --always)

.PHONY: build
build:
	GOARCH=wasm GOOS=wasip1 go build -o conduit-processor-textgen.wasm cmd/processor/main.go

.PHONY: test
test:
	go test $(GOTEST_FLAGS) -race ./...

.PHONY: generate
generate:
	go generate ./...

.PHONY: fmt
fmt:
	gofumpt -l -w .

.PHONY: lint
lint:
	golangci-lint run
