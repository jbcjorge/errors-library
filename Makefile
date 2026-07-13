.PHONY: tools test fmt vet lint vuln complexity shadow gosec gitleaks check clean

REPORTS_DIR := reports

## tools: Install development tools
tools:
	go install honnef.co/go/tools/cmd/staticcheck@latest
	go install golang.org/x/vuln/cmd/govulncheck@latest
	go install github.com/fzipp/gocyclo/cmd/gocyclo@latest
	go install github.com/boumenot/gocover-cobertura@latest
	go install golang.org/x/tools/go/analysis/passes/shadow/cmd/shadow@latest
	go install github.com/securego/gosec/v2/cmd/gosec@latest
	go install gotest.tools/gotestsum@latest

## fmt: Format code
fmt:
	gofmt -s -w .

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

## test: Run tests with coverage and JUnit XML output
test:
	@mkdir -p $(REPORTS_DIR)
	gotestsum --junitfile $(REPORTS_DIR)/junit.xml -- -count=1 -coverprofile=$(REPORTS_DIR)/coverage.out --covermode=count ./...
	@go tool cover -html=$(REPORTS_DIR)/coverage.out -o $(REPORTS_DIR)/coverage.html
	gocover-cobertura < $(REPORTS_DIR)/coverage.out > $(REPORTS_DIR)/coverage.xml

## check: Run all quality gates (the "is this ready to push?" command)
check: fmt vet shadow lint vuln gosec gitleaks complexity test

## shadow: Check for variable shadowing
shadow:
	go vet -vettool=$$(go env GOPATH)/bin/shadow ./...

## gosec: Security-focused static analysis
gosec:
	gosec -quiet ./...

## gitleaks: Scan for secrets in git history and working tree
gitleaks:
	gitleaks detect --no-git -v

## clean: Remove build artifacts and reports
clean:
	rm -rf $(REPORTS_DIR) build/
