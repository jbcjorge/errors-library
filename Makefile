.PHONY: tools update-tools update-deps test fmt fmt-check vet lint vuln complexity shadow gosec gitleaks mutation check clean

REPORTS_DIR := reports
MODULE := github.com/jbcjorge/errors-library

## tools: Install the pinned development tools (dev only; not module dependencies)
#
# Tool versions must track the Go toolchain: a static-analysis tool reads the
# compiler's export data, so a pin that lags the toolchain fails to load the
# standard library. Bump these together with the go directive in go.mod.
tools:
	go install honnef.co/go/tools/cmd/staticcheck@v0.8.2
	go install golang.org/x/vuln/cmd/govulncheck@v1.9.0
	go install github.com/fzipp/gocyclo/cmd/gocyclo@v0.6.0
	go install github.com/boumenot/gocover-cobertura@v1.5.0
	go install golang.org/x/tools/go/analysis/passes/shadow/cmd/shadow@v0.52.0
	go install github.com/securego/gosec/v2/cmd/gosec@v2.30.0
	go install gotest.tools/gotestsum@v1.13.0
	go install github.com/go-gremlins/gremlins/cmd/gremlins@v0.5.0

## update-tools: Install the latest version of every development tool
#
# Use this when bumping the Go toolchain, then copy the resolved versions back
# into the `tools` target to re-pin them.
update-tools:
	go install honnef.co/go/tools/cmd/staticcheck@latest
	go install golang.org/x/vuln/cmd/govulncheck@latest
	go install github.com/fzipp/gocyclo/cmd/gocyclo@latest
	go install github.com/boumenot/gocover-cobertura@latest
	go install golang.org/x/tools/go/analysis/passes/shadow/cmd/shadow@latest
	go install github.com/securego/gosec/v2/cmd/gosec@latest
	go install gotest.tools/gotestsum@latest
	go install github.com/go-gremlins/gremlins/cmd/gremlins@latest

## update-deps: Upgrade direct module dependencies and the Go directive, then verify
update-deps:
	@go list -m -f '{{if not .Indirect}}{{.Path}}{{end}}' all | grep -v '^$(MODULE)$$' | xargs -I{} go get {}@latest
	go get go@latest
	go mod tidy
	go build ./...
	go test -race -count=1 ./...
	@echo "Dependencies updated and verified. Review go.mod, then re-pin tools with 'make update-tools'."

## fmt: Format code
fmt:
	gofmt -s -w .

## fmt-check: Fail if any file is not formatted (does not modify files)
fmt-check:
	@unformatted=$$(gofmt -s -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "These files are not formatted, run 'make fmt':"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

## vet: Run go vet
vet:
	go vet ./...

## lint: Run staticcheck
lint:
	staticcheck ./...

## vuln: Run govulncheck
vuln:
	govulncheck ./...

## complexity: Check cyclomatic complexity (threshold: 15)
complexity:
	@output=$$(gocyclo -over 15 .); \
	if [ -n "$$output" ]; then \
		echo "Cyclomatic complexities over 15:"; \
		echo ""; \
		echo "$$output"; \
		exit 1; \
	fi
	@gocyclo -avg . | grep '^Average'

## test: Run tests with the race detector, coverage and JUnit XML output
test:
	@mkdir -p $(REPORTS_DIR)
	gotestsum --junitfile $(REPORTS_DIR)/junit.xml -- -race -count=1 -coverprofile=$(REPORTS_DIR)/coverage.out --covermode=atomic ./...
	@go tool cover -html=$(REPORTS_DIR)/coverage.out -o $(REPORTS_DIR)/coverage.html
	gocover-cobertura < $(REPORTS_DIR)/coverage.out > $(REPORTS_DIR)/coverage.xml

## check: Run all quality gates (the "is this ready to push?" command)
check: fmt-check vet shadow lint vuln gosec gitleaks complexity test mutation

## shadow: Check for variable shadowing
shadow:
	go vet -vettool=$$(go env GOPATH)/bin/shadow ./...

## gosec: Security-focused static analysis
gosec:
	gosec -quiet ./...

## gitleaks: Scan the working tree for secrets (git history is not scanned)
gitleaks:
	gitleaks detect --no-git -v

## mutation: Mutation testing quality gate (fails below the thresholds in .gremlins.yaml)
mutation:
	gremlins unleash .

## clean: Remove build artifacts and reports
clean:
	rm -rf $(REPORTS_DIR) build/
