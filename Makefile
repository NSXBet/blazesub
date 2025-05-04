.PHONY: test
test: gotestsum
	@gotestsum --format github-actions --format-icons=hivis -- --count=1 ./...

.PHONY: race
race: gotestsum
	@gotestsum --format github-actions --format-icons=hivis -- --race --count=1 ./...

.PHONY: bench
bench:
	@go test -bench=. -short -benchmem

.PHONY: lint
lint:
	@golangci-lint run --issues-exit-code=1 --fix

.PHONY: multiple-race
race-many: gotestsum
	@gotestsum --format github-actions --format-icons=hivis -- --race --count=20 ./...

.PHONY: gotestsum
gotestsum:
	@which gotestsum > /dev/null || (echo "gotestsum not found. Installing..." && go install gotest.tools/gotestsum@latest)
