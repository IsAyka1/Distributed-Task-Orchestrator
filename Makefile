export GOTOOLCHAIN := go$(shell sed -n 's/^go //p' go.mod)

.PHONY: check fmt check-fmt vet build test coverage

check: check-fmt vet build test

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

vet:
	go vet ./...

test:
	go test ./...

coverage:
	mkdir -p coverage
	go test -race -count=1 -covermode=atomic -coverpkg=./... -coverprofile=coverage/go.out ./...
	go tool cover -func=coverage/go.out > coverage/go.txt
	go tool cover -html=coverage/go.out -o coverage/go.html
	cat coverage/go.txt

.PHONY: test-python
test-python:
	python3 scripts/test_python.py
