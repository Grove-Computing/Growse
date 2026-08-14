STATICCHECK_VERSION ?= 2026.1
GOVULNCHECK_VERSION ?= v1.6.0
ACTIONLINT_VERSION ?= v1.7.12
PROJECT_GO := $(shell go env GOROOT)/bin/go

.PHONY: test race vet staticcheck vulncheck actionlint installer-test release-test fmt-check tidy-check quality ci

test:
	go test ./...

race:
	go test -race ./...

vet:
	go vet ./...

staticcheck:
	GOTOOLCHAIN=local $(PROJECT_GO) run honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VERSION) ./...

vulncheck:
	GOTOOLCHAIN=local $(PROJECT_GO) run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...

actionlint:
	GOTOOLCHAIN=local $(PROJECT_GO) run github.com/rhysd/actionlint/cmd/actionlint@$(ACTIONLINT_VERSION)

installer-test:
	bash tests/install.sh

release-test:
	bash tests/release.sh

fmt-check:
	@test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1)

tidy-check:
	go mod tidy -diff

quality: fmt-check tidy-check vet staticcheck actionlint

ci: quality race vulncheck installer-test release-test
