# Run all project checks
check: fmt-check vet test

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
  go run . {{quote(file)}}
