.PHONY: test
test:
	@gotestsum --format github-actions --format-icons=hivis --debug -- --count=1 ./...

.PHONY: bench
bench:
	@go test -bench=. -short -benchmem

.PHONY: lint
lint:
	@golangci-lint run --issues-exit-code=1 --fix
