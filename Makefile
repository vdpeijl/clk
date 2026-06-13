# Common developer tasks. CGO is disabled everywhere so the binary stays static.
.PHONY: build test fmt lint check tools hooks

build:
	CGO_ENABLED=0 go build ./cmd/clk/

test:
	CGO_ENABLED=0 go test ./...

# Format every tracked Go file in place.
fmt:
	gofmt -w $(shell git ls-files '*.go')

# Same invocation CI runs.
lint:
	golangci-lint run ./...

# Pre-push sanity: format, lint, test.
check: fmt lint test

# Install the linter version CI uses (golangci-lint @latest).
tools:
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Install the pre-commit hook without disturbing other hooks (e.g. clk's
# post-commit capture). Uses a symlink so updates to .githooks take effect.
hooks:
	ln -sf ../../.githooks/pre-commit .git/hooks/pre-commit
	@echo "Installed .git/hooks/pre-commit -> .githooks/pre-commit"
