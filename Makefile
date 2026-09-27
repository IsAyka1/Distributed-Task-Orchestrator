export GOTOOLCHAIN := go$(shell sed -n 's/^go //p' go.mod)

.PHONY: check fmt check-fmt build test

check: check-fmt build test

fmt:
	"$$(go env GOROOT)/bin/gofmt" -w .

check-fmt:
	@files=$$("$$(go env GOROOT)/bin/gofmt" -l .) || exit 1; \
	if [ -n "$$files" ]; then \
		printf 'Run make fmt to format:\n%s\n' "$$files"; \
		exit 1; \
	fi

build:
	go build -o bin/orchestrator ./cmd/orchestrator

test:
	go test ./...
