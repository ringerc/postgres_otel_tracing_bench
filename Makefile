# Convenience wrapper around `go build`/`go test`. The real entry point
# is `go run ./cmd/otelbench ...` --- this Makefile is just a place to
# anchor the build-tag variants.

.PHONY: all build build-patched test clean fmt vet

all: build

build:
	go build -o otelbench ./cmd/otelbench

# Build with the patched-pgx fork (requires the go.mod replace directive
# to be pointing at a local checkout of ringerc/pgx_patches).
build-patched:
	go build -tags=patched_pgx -o otelbench-patched ./cmd/otelbench

test:
	go test ./...

fmt:
	go fmt ./...

vet:
	go vet ./...
	go vet -tags=patched_pgx ./...

clean:
	rm -f otelbench otelbench-patched
