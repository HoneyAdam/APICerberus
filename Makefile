BINARY := apicerberus
BIN_DIR := bin
MAIN := ./cmd/apicerberus
WEB_DIR := web

VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -X github.com/APICerberus/APICerebrus/internal/version.Version=$(VERSION) \
	-X github.com/APICerberus/APICerebrus/internal/version.Commit=$(COMMIT) \
	-X github.com/APICerberus/APICerebrus/internal/version.BuildTime=$(BUILD_TIME)

.PHONY: build clean test lint web-build benchmark coverage race integration e2e docker security backup restore deploy runbook ci \
    docker-build docker-push docker-compose-up docker-compose-down docker-compose-logs docker-compose-prod-up docker-compose-prod-down \
    deploy-k8s deploy-k8s-dev deploy-k8s-staging deploy-k8s-prod \
    release release-dry-run ci-full

web-build:
	@if [ -f $(WEB_DIR)/package.json ]; then \
		cd $(WEB_DIR) && npm ci && npm run build; \
	fi

build: web-build
	@mkdir -p $(BIN_DIR)
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY) $(MAIN)

clean:
	rm -rf $(BIN_DIR)
	rm -rf coverage/

test: web-build
	go test ./...

test-race:
	go test -race ./...

test-v:
	go test -v ./...

benchmark:
	go test -bench=. -benchmem ./test/benchmark/...
	go test -bench=. -benchmem -run=^$$ ./internal/...

coverage:
	@mkdir -p coverage
	go test -race -coverprofile=coverage/coverage.out -covermode=atomic ./...
	go tool cover -html=coverage/coverage.out -o coverage/coverage.html
	@echo "Coverage report generated: coverage/coverage.html"
	@go tool cover -func=coverage/coverage.out | tail -1

coverage-report: coverage
	@echo "Opening coverage report..."
	@if command -v xdg-open >/dev/null; then xdg-open coverage/coverage.html; \
	elif command -v open >/dev/null; then open coverage/coverage.html; \
	elif command -v start >/dev/null; then start coverage/coverage.html; \
	fi

integration:
	go test -tags=integration ./test/...

e2e:
	go test -tags=e2e ./test/...

lint: web-build
	go vet ./...
	@if command -v golangci-lint >/dev/null; then golangci-lint run; fi

fmt:
	go fmt ./...

fmt-check:
	@if [ -n "$$(go fmt ./...)" ]; then echo "Code is not formatted"; exit 1; fi

deps:
	go mod download
	go mod verify

deps-update:
	go get -u ./...
	go mod tidy

docker:
	@bash scripts/build-docker.sh --tag $(VERSION)

docker-push:
	@bash scripts/build-docker.sh --tag $(VERSION) --push

docker-compose-up:
	@docker-compose -f docker-compose.yml up -d

docker-compose-down:
	@docker-compose -f docker-compose.yml down

docker-compose-logs:
	@docker-compose -f docker-compose.yml logs -f

docker-compose-prod-up:
	@docker-compose -f docker-compose.prod.yml up -d

docker-compose-prod-down:
	@docker-compose -f docker-compose.prod.yml down

security:
	@echo "Running security scans..."
	@bash scripts/security-scan.sh

security-scan: security

security-gosec:
	@if command -v gosec >/dev/null; then \
		echo "Running gosec..."; \
		gosec -exclude-generated ./...; \
	else \
		echo "gosec not installed. Install with: go install github.com/securego/gosec/v2/cmd/gosec@latest"; \
		exit 1; \
	fi

security-vuln:
	@if command -v govulncheck >/dev/null; then \
		echo "Running govulncheck..."; \
		govulncheck ./...; \
	else \
		echo "govulncheck not installed. Install with: go install golang.org/x/vuln/cmd/govulncheck@latest"; \
		exit 1; \
	fi

security-trivy:
	@if command -v trivy >/dev/null; then \
		echo "Running trivy filesystem scan..."; \
		trivy fs --scanners vuln,secret,misconfig .; \
	else \
		echo "trivy not installed. See: https://aquasecurity.github.io/trivy/"; \
		exit 1; \
	fi

# Operations targets
backup:
	@echo "Creating backup..."
	@bash scripts/backup.sh

restore:
	@if [ -z "$(BACKUP_FILE)" ]; then \
		echo "Error: BACKUP_FILE not set"; \
		echo "Usage: make restore BACKUP_FILE=backups/apicerberus_backup_xxx.tar.gz"; \
		exit 1; \
	fi
	@bash scripts/restore.sh $(BACKUP_FILE)

deploy-swarm:
	@echo "Deploying to Docker Swarm..."
	@docker stack deploy -c deployments/docker/docker-compose.swarm.yml apicerberus

deploy-k8s:
	@echo "Deploying to Kubernetes..."
	@bash scripts/deploy-k8s.sh $(ENV)

deploy-k8s-dev:
	@bash scripts/deploy-k8s.sh development

deploy-k8s-staging:
	@bash scripts/deploy-k8s.sh staging

deploy-k8s-prod:
	@bash scripts/deploy-k8s.sh production

# Docker targets
docker-build:
	@bash scripts/build-docker.sh --tag $(VERSION)

docker-build-push:
	@bash scripts/build-docker.sh --tag $(VERSION) --push

docker-compose-up:
	@docker-compose -f docker-compose.yml up -d

docker-compose-down:
	@docker-compose -f docker-compose.yml down

docker-compose-logs:
	@docker-compose -f docker-compose.yml logs -f

docker-compose-prod-up:
	@docker-compose -f docker-compose.prod.yml up -d

docker-compose-prod-down:
	@docker-compose -f docker-compose.prod.yml down

# Release targets
release:
	@if [ -z "$(VERSION)" ]; then \
		echo "Error: VERSION is required"; \
		echo "Usage: make release VERSION=v1.0.0"; \
		exit 1; \
	fi
	@bash scripts/release.sh $(VERSION)

release-dry-run:
	@if [ -z "$(VERSION)" ]; then \
		echo "Error: VERSION is required"; \
		echo "Usage: make release-dry-run VERSION=v1.0.0"; \
		exit 1; \
	fi
	@bash scripts/release.sh --dry-run $(VERSION)

# CI/CD targets
ci: fmt lint test-race security coverage
	@echo "CI checks complete"

ci-full: ci integration e2e
	@echo "Full CI pipeline complete"

# Health and metrics
health:
	@curl -f http://localhost:8080/health || echo "Health check failed"

metrics:
	@curl -s http://localhost:8080/metrics | head -50

changelog:
	@git log --pretty=format:"- %s (%h)" $(shell git describe --tags --abbrev=0 2>/dev/null || echo HEAD~10)..HEAD

all: fmt lint test-race build
