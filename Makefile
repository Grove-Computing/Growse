STATICCHECK_VERSION ?= 2026.1
GOVULNCHECK_VERSION ?= v1.6.0
ACTIONLINT_VERSION ?= v1.7.12
PROJECT_GO := $(shell go env GOROOT)/bin/go

.PHONY: test race examples-test v014-conformance-test v015-framework-test v015-visual-test v015-conformance-test v015-security-test vet staticcheck vulncheck actionlint securityscan installer-test release-test docker-test supply-chain-test developer-security-test skills-test docs-test fmt-check tidy-check quality ci

test:
	go test ./...

race:
	go test -race ./...

examples-test:
	bash tests/examples.sh

v014-conformance-test:
	bash tests/v014-conformance.sh

v015-framework-test:
	bash tests/v015-framework.sh

v015-visual-test:
	bash tests/v015-visual.sh

v015-conformance-test:
	bash tests/v015-conformance.sh

v015-security-test:
	bash tests/v015-security.sh

vet:
	go vet ./...

staticcheck:
	GOTOOLCHAIN=local $(PROJECT_GO) run honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VERSION) ./...

vulncheck:
	GOTOOLCHAIN=local $(PROJECT_GO) run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...

actionlint:
	GOTOOLCHAIN=local $(PROJECT_GO) run github.com/rhysd/actionlint/cmd/actionlint@$(ACTIONLINT_VERSION)

securityscan:
	go run ./cmd/securityscan

installer-test:
	bash tests/install.sh
	bash tests/install-platforms.sh

release-test:
	bash tests/release.sh

docker-test:
	bash tests/docker.sh

supply-chain-test:
	bash tests/supply-chain.sh

developer-security-test:
	bash tests/developer-security.sh

skills-test:
	bash tests/skills.sh

docs-test:
	bash tests/docs.sh

fmt-check:
	@test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1)

tidy-check:
	go mod tidy -diff

quality: fmt-check tidy-check vet staticcheck actionlint

ci: quality securityscan race examples-test v014-conformance-test v015-framework-test v015-visual-test v015-conformance-test v015-security-test vulncheck installer-test release-test docker-test supply-chain-test developer-security-test skills-test docs-test
