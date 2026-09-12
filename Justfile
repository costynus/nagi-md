# Run all project checks
check: fmt-check tidy-check vet test

# Check Go formatting without modifying files
fmt-check:
    #!/usr/bin/env sh
    set -eu

    unformatted_files=$(gofmt -l .)
    if [ -n "$unformatted_files" ]; then
      echo "The following files are not properly formatted:"
      echo "$unformatted_files"
      exit 1
    fi

# Check that go.mod and go.sum are tidy
tidy-check:
    go mod tidy --diff

# Format Go source files
fmt:
    gofmt -w .

# Run static analysis
vet:
    go vet ./...

# Run tests
test:
    go test ./...

# Run the editor with a Markdown file
run file:
    go run . {{ quote(file) }}

# Build the project
build:
    go build -o nmd .
