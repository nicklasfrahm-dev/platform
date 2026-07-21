################
### OpenTofu ###
################

OPENTOFU_ROOT_MODULE	?= deploy/opentofu
OPENTOFU_PLAN					?= $(OPENTOFU_ROOT_MODULE)/opentofu.tfplan
OPENTOFU_LOG					?= $(OPENTOFU_ROOT_MODULE)/tofu.log
OPENTOFU_ROOT_SOURCES	?= $(shell find $(OPENTOFU_ROOT_MODULE) -maxdepth 1 -type f -name '*.tf')

# The default SHELL (dash) has no `pipefail`, so `tofu plan | tee log`
# would report success from `tee` even when `tofu plan` itself fails. Scope
# a bash+pipefail shell to just this target so a failing plan actually
# fails the build instead of silently producing an empty/errored plan file.
$(OPENTOFU_PLAN): SHELL := bash
$(OPENTOFU_PLAN): .SHELLFLAGS := -eu -o pipefail -c

# OPENTOFU_ROOT_MODULE is overridable (e.g. `make opentofu-plan
# OPENTOFU_ROOT_MODULE=deploy/infra/prd-cloudrun-services`) so the same
# targets drive every root module under deploy/, not just deploy/opentofu.
$(OPENTOFU_PLAN): $(OPENTOFU_ROOT_SOURCES)
	tofu -chdir=$(OPENTOFU_ROOT_MODULE) init
	tofu -chdir=$(OPENTOFU_ROOT_MODULE) plan -out=opentofu.tfplan | tee $(OPENTOFU_LOG)
	@sed -i 's/\x1b\[[0-9;]*m//g' $(OPENTOFU_LOG)

opentofu-plan: $(OPENTOFU_PLAN) ## Plan the infrastructure changes.

.PHONY: opentofu-count
opentofu-count: opentofu-plan ## Count the number of changes in the plan.
	@tofu -chdir=$(OPENTOFU_ROOT_MODULE) show -json opentofu.tfplan | jq -r '(.resource_changes // [])[].change.actions | join(",")' | grep -Ecv '^no-op$$' || true

.PHONY: opentofu-apply
opentofu-apply: ## Apply the infrastructure changes.
	tofu -chdir=$(OPENTOFU_ROOT_MODULE) init
	tofu -chdir=$(OPENTOFU_ROOT_MODULE) apply -auto-approve

################
### Monorepo ###
################

MODULE     := github.com/nicklasfrahm/platform
BIN_DIR    := bin

GIT_TAG    := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := \
  -X $(MODULE)/pkg/probe.Version=$(GIT_TAG) \
  -X $(MODULE)/pkg/probe.Commit=$(GIT_COMMIT) \
  -X $(MODULE)/pkg/probe.BuildTime=$(BUILD_TIME)

SERVICES := $(notdir $(wildcard cmd/*))

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help message
	@printf "\033[1mplatform — Go monorepo\033[0m\n\n"
	@printf "\033[36mUsage:\033[0m\n  make \033[36m<target>\033[0m\n\n"
	@awk '/^##@/ { \
		printf "\n\033[1m%s:\033[0m\n", substr($$0, 5) \
	} /^##> / { \
		n = index($$0, " ## "); \
		printf "  \033[36m%-24s\033[0m %s\n", substr($$0, 5, n - 5), substr($$0, n + 4) \
	} /^[a-zA-Z/_-]+:.*?## / { \
		split($$0, a, ":.*?## "); \
		printf "  \033[36m%-24s\033[0m %s\n", a[1], a[2] \
	}' $(MAKEFILE_LIST)

##@ Development

.PHONY: dev
dev: ## Start all services via Docker Compose (--profile dev)
	docker compose --profile dev up

.PHONY: generate
generate: ## Run go generate for all packages
	go generate ./...

##@ Build

##> build/<name> ## Build a service binary
.PHONY: $(addprefix build/,$(SERVICES))
$(addprefix build/,$(SERVICES)): build/%: generate
	@mkdir -p $(BIN_DIR)
	go build -ldflags="$(LDFLAGS)" -o $(BIN_DIR)/$* ./cmd/$*

.PHONY: build
build: $(addprefix build/,$(SERVICES)) ## Build all service binaries

##> run/<name> ## Build and run a service
.PHONY: $(addprefix run/,$(SERVICES))
$(addprefix run/,$(SERVICES)): run/%: build/%
	$(BIN_DIR)/$*

##@ Quality

.PHONY: test
test: generate ## Run the full test suite
	go test ./...

.PHONY: lint
lint: ## Run golangci-lint
	golangci-lint run ./...

##@ Maintenance

.PHONY: tidy
tidy: ## Tidy and verify go module dependencies
	go mod tidy
	go mod verify

.PHONY: tools
tools: ## Install development tools (golangci-lint)
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest

.PHONY: clean
clean: ## Remove build artefacts
	rm -rf $(BIN_DIR)
