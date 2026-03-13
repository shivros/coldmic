SHELL := /bin/bash

STATICCHECK_VERSION ?= v0.7.0
WAILS_VERSION ?= v2.11.0
WAILS_CLI ?= go run github.com/wailsapp/wails/v2/cmd/wails@$(WAILS_VERSION)
WAILS_BUILD_ARGS ?=
CLI_EXE_SUFFIX ?=
GO_COVERAGE_MIN ?= 74
FRONTEND_COVERAGE_LINES_MIN ?= 85
FRONTEND_COVERAGE_STATEMENTS_MIN ?= 85
FRONTEND_COVERAGE_FUNCTIONS_MIN ?= 80
FRONTEND_COVERAGE_BRANCHES_MIN ?= 60
GO_PACKAGES ?= $(shell go list ./... | grep -v '/frontend/node_modules/')

.PHONY: build build-app build-cli install-cli dev test test-go test-go-race test-go-coverage ci-test-go \
	test-frontend test-frontend-coverage \
	lint lint-go lint-frontend gofmt-check go-vet staticcheck \
	quality quality-go quality-frontend frontend-build \
	ci-deps-frontend ci-quality-frontend ci-test-frontend \
	build-app-ci build-app-ci-linux ci-build-cli ci-build-desktop ci-build-desktop-linux ci-build-all \
	verify-ci-contract ci clean

build: build-app build-cli

build-app:
	wails build

build-app-ci:
	$(WAILS_CLI) build -clean $(WAILS_BUILD_ARGS)

build-app-ci-linux: WAILS_BUILD_ARGS := -tags webkit2_41
build-app-ci-linux: build-app-ci

build-cli:
	mkdir -p build/bin
	go build -o build/bin/coldmic$(CLI_EXE_SUFFIX) ./cmd/coldmic
	go build -o build/bin/coldmicd$(CLI_EXE_SUFFIX) ./cmd/coldmicd

install-cli:
	go install ./cmd/coldmic
	go install ./cmd/coldmicd

dev:
	wails dev

test:
	$(MAKE) test-go

test-go:
	go test $(GO_PACKAGES)

test-go-race:
	go test -race $(GO_PACKAGES)

test-go-coverage:
	go test -coverprofile=coverage.out $(GO_PACKAGES)
	@total=$$(go tool cover -func=coverage.out | awk '/^total:/ {gsub("%","",$$3); print $$3}'); \
	echo "Go coverage: $$total% (required >= $(GO_COVERAGE_MIN)%)"; \
	awk -v total="$$total" -v min="$(GO_COVERAGE_MIN)" 'BEGIN { exit (total+0 < min+0) ? 1 : 0 }' || \
	( echo "Go coverage threshold not met"; exit 1 )

ci-test-go: test-go-race test-go-coverage

test-frontend:
	cd frontend && npm run test

test-frontend-coverage:
	cd frontend && npm run test:coverage
	FRONTEND_COVERAGE_LINES_MIN=$(FRONTEND_COVERAGE_LINES_MIN) \
	FRONTEND_COVERAGE_STATEMENTS_MIN=$(FRONTEND_COVERAGE_STATEMENTS_MIN) \
	FRONTEND_COVERAGE_FUNCTIONS_MIN=$(FRONTEND_COVERAGE_FUNCTIONS_MIN) \
	FRONTEND_COVERAGE_BRANCHES_MIN=$(FRONTEND_COVERAGE_BRANCHES_MIN) \
	node -e 'const fs = require("fs"); const summary = JSON.parse(fs.readFileSync("frontend/coverage/coverage-summary.json", "utf8")); const total = summary.total; const mins = { lines: Number(process.env.FRONTEND_COVERAGE_LINES_MIN), statements: Number(process.env.FRONTEND_COVERAGE_STATEMENTS_MIN), functions: Number(process.env.FRONTEND_COVERAGE_FUNCTIONS_MIN), branches: Number(process.env.FRONTEND_COVERAGE_BRANCHES_MIN) }; const fields = ["lines", "statements", "functions", "branches"]; const failures = fields.filter((field) => total[field].pct < mins[field]); fields.forEach((field) => { console.log("Frontend coverage " + field + ": " + total[field].pct + "% (required >= " + mins[field] + "%)"); }); if (failures.length) { console.error("Frontend coverage threshold not met for: " + failures.join(", ")); process.exit(1); }'

lint: lint-go lint-frontend

lint-go: gofmt-check go-vet staticcheck

gofmt-check:
	@unformatted=$$(gofmt -l $$(find . -type f -name '*.go' -not -path './frontend/*' -not -path './vendor/*')); \
	if [ -n "$$unformatted" ]; then \
		echo "The following files are not gofmt-formatted:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

go-vet:
	go vet $(GO_PACKAGES)

staticcheck:
	go run honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VERSION) $(GO_PACKAGES)

lint-frontend:
	cd frontend && npm run lint

quality: quality-go quality-frontend

quality-go: lint-go

quality-frontend: lint-frontend frontend-build

frontend-build:
	cd frontend && npm run build

ci-deps-frontend:
	cd frontend && npm ci

ci-quality-frontend: ci-deps-frontend quality-frontend

ci-test-frontend: ci-deps-frontend test-frontend-coverage

ci-build-cli: build-cli

ci-build-desktop: ci-deps-frontend build-app-ci

ci-build-desktop-linux: ci-deps-frontend build-app-ci-linux

ci-build-all: ci-build-cli ci-build-desktop

verify-ci-contract:
	bash scripts/ci/verify_ci_contract.sh

ci: quality-go ci-quality-frontend ci-test-go ci-test-frontend verify-ci-contract

clean:
	rm -f build/bin/coldmic build/bin/coldmicd build/bin/coldmic-desktop build/bin/coldmic.exe build/bin/coldmicd.exe
