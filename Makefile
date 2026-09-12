# GOEXE is ".exe" on Windows, "" elsewhere -- naming.md requires the
# Windows binary to carry that extension (without it, Windows won't run
# it by bare name). `go build -o san-db-ox` alone does NOT add this
# automatically: unlike its default (no -o) naming behavior, `-o` is used
# verbatim (see `go help build`).
BIN := san-db-ox$(shell go env GOEXE)

.PHONY: build unit test check fmt fmt-check vet netcheck race clean

# Builds $(BIN) to the repo root, and (via the same dependency graph)
# verifies engine compiles too. `go build ./...` does NOT do this: for
# more than one package it only checks compilability and discards the
# output (see `go help build`), so the binary must be built explicitly by
# import path.
build:
	go build -o $(BIN) ./cmd/san-db-ox

# Go unit tests (engine package, footer I/O, etc).
unit:
	go test ./...

# E2E/integration tests against a freshly built binary (tests/e2e.sh,
# .claude/rules/testing.md). Depends on build so `make test` alone is
# always enough.
test: build
	bash tests/e2e.sh

# Fast pre-commit / CI gate: format, vet, no net/net-http dependency,
# compile, unit tests. Deliberately excludes e2e (test) and race (slow,
# separate jobs/targets).
check: fmt-check vet netcheck build unit

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

# Enforces .claude/rules/directory-structure.md: engine and cmd/san-db-ox
# must not import net/net-http directly, and net/http must not appear
# even transitively. `net` itself IS expected transitively (modernc.org/
# sqlite -> modernc.org/libc), so it is deliberately not checked here
# (.claude/rules/testing.md: "「netが一切現れない」という検証は失敗する").
netcheck:
	@if go list -f '{{join .Imports "\n"}}' ./engine ./cmd/san-db-ox | grep -qx 'net\|net/http'; then \
		echo "engine and cmd/san-db-ox must not directly import net or net/http" 1>&2; \
		exit 1; \
	fi
	@if go list -deps ./engine ./cmd/san-db-ox | grep -qx 'net/http'; then \
		echo "net/http must not appear even transitively" 1>&2; \
		exit 1; \
	fi

# Requires a C compiler (CGO_ENABLED=1); not part of `check`. See
# .claude/rules/testing.md "-race の運用方針".
race:
	CGO_ENABLED=1 go test -race -count=1 ./...

clean:
	rm -f san-db-ox san-db-ox.exe
