####################################################################################################
# Global ###########################################################################################
####################################################################################################

OS := $(shell uname -s | tr '[:upper:]' '[:lower:]')
ARCH := $(shell uname -m)

USER_UID := $(shell id -u)
USER_GID := $(shell id -g)

KUBERNETES_VERSION := 1.29.5
KUBECONFIG ?= $(HOME)/.kube/config

####################################################################################################
# Docker ###########################################################################################
####################################################################################################

DOCKER_CLIENT_VERSION := $(shell docker version --format '{{.Client.Version}}')
DOCKER_KUBE_CONFIG := -v $(KUBECONFIG):/.kube/config -e KUBECONFIG=/.kube/config
DOCKER_USER := --user $(USER_UID):$(USER_GID)
DOCKER_RUN := docker run --rm -v $(PWD):/app -w /app --network host $(DOCKER_USER) $(DOCKER_KUBE_CONFIG)

####################################################################################################
# Kubectl ##########################################################################################
####################################################################################################

KUBECTL_DOCKER_VERSION := 1.30.1
KUBECTL_DOCKER_RUN := $(DOCKER_RUN) bitnami/kubectl:$(KUBECTL_DOCKER_VERSION)

####################################################################################################
# GoLang ###########################################################################################
####################################################################################################

GOMODCACHE ?= $(HOME)/go/pkg/mod
GOCACHE ?= $(HOME)/.cache/go-build

GO_DOCKER_VERSION := 1
GO_DOCKER_CACHES := -v $(GOMODCACHE):/go/pkg/mod -e GOMODCACHE=/go/pkg/mod -v $(GOCACHE):/go/.cache/go-build -e GOCACHE=/go/.cache/go-build
GO_DOCKER_RUN := $(DOCKER_RUN) $(GO_DOCKER_CACHES) -e CGO_ENABLED=0 golang:$(GO_DOCKER_VERSION) go

####################################################################################################
# Clean ############################################################################################
####################################################################################################

.PHONY: clean
clean: test_clean minikube_clean
	@echo "Clean finished."

####################################################################################################
# Lint #############################################################################################
####################################################################################################

GOLANGCI_LINT_VERSION := v1.59
GOLANGCI_LINT_DOCKER := $(DOCKER_RUN) -e GOLANGCI_LINT_CACHE=/app/.cache/golangci-lint $(GO_DOCKER_CACHES) golangci/golangci-lint:$(GOLANGCI_LINT_VERSION) golangci-lint

.PHONY: lint_go
lint_go:
	@echo "Linting go files."
	@$(GOLANGCI_LINT_DOCKER) run --out-format colored-line-number --max-issues-per-linter 3

.PHONY: lint
lint: lint_go helm_lint_charts
	@echo "Linting finished."

####################################################################################################
# Tests ############################################################################################
####################################################################################################

.PHONY: test_clean
test_clean:
	@echo "Cleaning test cache."
	@$(GO_DOCKER_RUN) clean -testcache

.PHONY: test_unit
test_unit: test_clean
	@echo "Running unit tests with coverage report."
	@$(GO_DOCKER_RUN) test ./pkg/... -coverprofile=.unit_tests_coverage.out
	@$(GO_DOCKER_RUN) tool cover -html=.unit_tests_coverage.out -o .unit_tests_coverage.html

.PHONY: test_unit_coverage_report
test_unit_coverage_report:
	@echo "Opening the unit tests coverage report in a web browser."
	@open .unit_tests_coverage.html

.PHONY: test
test: test_unit
	@echo "Test suites completed."

####################################################################################################
# Minikube #########################################################################################
####################################################################################################

MINIKUBE_PROFILE := --profile=intelligence
MINIKUBE_VERSION := v1.33.1
MINIKUBE_IMAGE_NAME := minikube-$(OS)-$(ARCH):$(MINIKUBE_VERSION)
MINIKUBE_DOCKER_SOCK := -v /var/run/docker.sock:/var/run/docker.sock
MINIKUBE_DOCKER_RUN := $(DOCKER_RUN) $(MINIKUBE_DOCKER_SOCK) $(MINIKUBE_IMAGE_NAME)

.PHONY: minikube_clean
minikube_clean:git
	@echo "Cleaning minikube."
	@rm -rf .minikube
	@docker rmi $(MINIKUBE_IMAGE_NAME) || true

.PHONY: minikube_build_image
minikube_build_image:
	@echo "Creating the minikube data folder."
	@mkdir -p .minikube
	@echo "Creating the image $(MINIKUBE_IMAGE_NAME)."
	@docker build images/minikube/ \
		--build-arg DOCKER_CLIENT_VERSION=$(DOCKER_CLIENT_VERSION) \
		--build-arg MINIKUBE_VERSION=$(MINIKUBE_VERSION) \
		--build-arg ARCH=$(ARCH) \
		--tag $(MINIKUBE_IMAGE_NAME) \
		--quiet

.PHONY: minikube_delete
minikube_delete: minikube_build_image
	@echo "Deleting this projects minikube cluster."
	@$(MINIKUBE_DOCKER_RUN) delete $(MINIKUBE_PROFILE)

.PHONY: minikube_start
minikube_start: minikube_build_image
	@echo "Starting a minikube cluster for this project."
	$(MINIKUBE_DOCKER_RUN) start $(MINIKUBE_PROFILE) \
		--kubernetes-version=$(KUBERNETES_VERSION) \
		--driver=docker \
		--memory=2g \
		--cpus=2 \
		--interactive=false \
		--nodes=3 \
		--cni=false \
		--network-plugin=cni \
		--extra-config=kubeadm.pod-network-cidr=192.168.0.0/16 \
		--subnet=172.16.0.0/24 \
		--embed-certs

####################################################################################################
# Helm #############################################################################################
####################################################################################################

HELM_ENV ?= local
HELM_VERSION := 3.14.4
HELM_CHART_PATHS := $(wildcard charts/*)
HELM_LINT_CHART_TARGETS := $(addprefix helm_lint_chart_, $(notdir $(HELM_CHART_PATHS)))
HELM_UPGRADE_DEFAULT_ARGS := --values charts/cni/values.yaml --values charts/cni/values-$(HELM_ENV).yaml --install --atomic --cleanup-on-fail --wait --timeout 30m0s --qps 5 --history-max 3
HELM_DOCKER_RUN := $(DOCKER_RUN) alpine/helm:$(HELM_VERSION)

.PHONY: $HELM_LINT_CHART_TARGETS
helm_lint_chart_%:
	@echo "Linting helm chart $*."
	@$(HELM_DOCKER_RUN) lint "charts/$*" --with-subcharts --quiet --strict

.PHONY: helm_lint_charts
helm_lint_charts: $(HELM_LINT_CHART_TARGETS)
	@echo "Linting helm charts finished."

.PHONY: helm_install_chart_cni
helm_install_chart_cni:
	@echo "Updating the helm chart dependencies for cni."
	@$(HELM_DOCKER_RUN) dependency update "charts/cni"
	@echo "Installing the helm chart cni."
	@$(HELM_DOCKER_RUN) upgrade cni charts/cni $(HELM_UPGRADE_DEFAULT_ARGS) --namespace kube-system
	@echo "Waiting for cilium to be ready."
	@$(KUBECTL_DOCKER_RUN) wait --for=condition=ready pod -l k8s-app=cilium --namespace kube-system

.PHONY: helm_install_chart_cni_default_policy
helm_install_chart_cni_default_policy:
	@echo "Installing helm chart cni-default-policy."
	@$(HELM_DOCKER_RUN) upgrade cni-default-policy charts/cni-default-policy $(HELM_UPGRADE_DEFAULT_ARGS)

.PHONY: helm_install_rook_ceph
helm_install_rook_ceph:
	@echo "Updating the helm chart dependencies for rook-ceph."
	@$(HELM_DOCKER_RUN) dependency update charts/rook-ceph
	@echo "Installing helm chart for rook-ceph."
	@$(HELM_DOCKER_RUN) upgrade rook-ceph charts/rook-ceph $(HELM_UPGRADE_DEFAULT_ARGS) --create-namespace --namespace rook-ceph

.PHONY: helm_install_charts
helm_install_charts: helm_install_chart_cni helm_install_chart_cni_default_policy helm_install_rook_ceph
	@echo "Finished installing helm charts."
