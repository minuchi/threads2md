.PHONY: build test lint fmt-check tidy-check smoke sanity clean

BIN := ./bin/threads2md

build:
	go build -o $(BIN) ./cmd/threads2md

test:
	go test ./... -race -cover

lint:
	go vet ./...

fmt-check:
	@out=$$(gofmt -l .); \
	if [ -n "$$out" ]; then \
	  echo "unformatted files:"; echo "$$out"; exit 1; \
	fi

tidy-check:
	@cp go.mod go.mod.bak
	@[ -f go.sum ] && cp go.sum go.sum.bak || true
	@go mod tidy
	@if ! diff -q go.mod go.mod.bak >/dev/null; then \
	  mv go.mod.bak go.mod; \
	  [ -f go.sum.bak ] && mv go.sum.bak go.sum || true; \
	  echo "go.mod out of date — run 'go mod tidy'"; exit 1; \
	fi
	@if [ -f go.sum.bak ] && ! diff -q go.sum go.sum.bak >/dev/null; then \
	  mv go.sum.bak go.sum; \
	  echo "go.sum out of date — run 'go mod tidy'"; exit 1; \
	fi
	@rm -f go.mod.bak go.sum.bak

smoke: build
	./scripts/smoke.sh

sanity: fmt-check tidy-check lint test build smoke
	@echo "sanity passed"

clean:
	rm -rf ./bin
