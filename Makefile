.PHONY: test
test:
	go test -v -cover -count=1 ./...

GOLANGCI_LINT_VERSION?=2.4.0

.PHONY: install_golangci_lint
install_golangci_lint:
	@echo golangci-lint $(GOLANGCI_LINT_VERSION) installing...
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b $$(go env GOPATH)/bin v$(GOLANGCI_LINT_VERSION)

.PHONY: lint
lint:
	@if ! golangci-lint --version | grep -q $(GOLANGCI_LINT_VERSION); then \
		$(MAKE) install_golangci_lint; \
	fi
	golangci-lint run
	@echo -e "\033[0;32mLinting passed 🎉\033[0m"