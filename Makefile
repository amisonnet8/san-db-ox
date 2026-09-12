.PHONY: build unit test check fmt fmt-check vet race clean

# Builds the san-db-ox binary to the repo root, and (via the same
# dependency graph) verifies engine compiles too. `go build ./...` does
# NOT do this: for more than one package it only checks compilability and
# discards the output (see `go help build`), so the binary must be built
# explicitly by import path.
build:
	go build -o san-db-ox ./cmd/san-db-ox

# Go unit tests (engine package, footer I/O, etc).
unit:
	go test ./...

# E2E/integration tests against a built binary (see tests/, .claude/rules/testing.md).
test:
	./tests/e2e.sh

# Fast pre-commit / CI gate: format, vet, compile, unit tests. Deliberately
# excludes e2e (test) and race (slow, separate jobs).
check: fmt-check vet build unit

fmt:
	gofmt -s -w .

fmt-check:
	@files="$$(gofmt -s -l .)"; \
	if [ -n "$$files" ]; then \
		echo "gofmt -s needed on:" 1>&2; \
		echo "$$files" 1>&2; \
		exit 1; \
	fi

vet:
	go vet ./...

# Requires a C compiler (CGO_ENABLED=1); not part of `check`. See
# .claude/rules/testing.md "-race の運用方針".
race:
	CGO_ENABLED=1 go test -race -count=1 ./...

clean:
	rm -f san-db-ox san-db-ox.exe
