TAG ?= $(shell git describe --tags --always)
ORG := keyval
BUILD_ARG ?=

.PHONY: build-odiglet
build-odiglet:
	docker build ${BUILD_ARG}  -t $(ORG)/odigos-odiglet:$(TAG) . -f odiglet/Dockerfile --build-arg ODIGOS_VERSION=$(TAG)

.PHONY: build-instrumentor
build-instrumentor:
	docker build ${BUILD_ARG} -t $(ORG)/odigos-instrumentor:$(TAG) . --build-arg SERVICE_NAME=instrumentor

.PHONY: build-images
build-images:
	make build-odiglet TAG=$(TAG)
	make build-instrumentor TAG=$(TAG)

.PHONY: push-odiglet
push-odiglet:
	docker buildx build --platform linux/amd64,linux/arm64/v8 --push -t $(ORG)/odigos-odiglet:$(TAG) . -f odiglet/Dockerfile

.PHONY: push-instrumentor
push-instrumentor:
	docker buildx build --platform linux/amd64,linux/arm64/v8 --push -t $(ORG)/odigos-instrumentor:$(TAG) . --build-arg SERVICE_NAME=instrumentor

.PHONY: push-images
push-images:
	make push-odiglet TAG=$(TAG)
	make push-instrumentor TAG=$(TAG)

.PHONY: load-to-kind-odiglet
load-to-kind-odiglet:
	kind load docker-image $(ORG)/odigos-odiglet:$(TAG)

.PHONY: load-to-kind-instrumentor
load-to-kind-instrumentor:
	kind load docker-image $(ORG)/odigos-instrumentor:$(TAG)

.PHONY: load-to-kind
load-to-kind:
	make load-to-kind-odiglet TAG=$(TAG)
	make load-to-kind-instrumentor TAG=$(TAG)

.PHONY: restart-odiglet
restart-odiglet:
	kubectl rollout restart daemonset odiglet -n odigos-system

.PHONY: restart-instrumentor
restart-instrumentor:
	kubectl rollout restart deployment odigos-instrumentor -n odigos-system

.PHONY: deploy-odiglet
deploy-odiglet:
	make build-odiglet TAG=$(TAG) && make load-to-kind-odiglet TAG=$(TAG) && make restart-odiglet

.PHONY: deploy-instrumentor
deploy-instrumentor:
	make build-instrumentor TAG=$(TAG) && make load-to-kind-instrumentor TAG=$(TAG) && make restart-instrumentor

.PHONY: debug-odiglet
debug-odiglet:
	docker build -t $(ORG)/odigos-odiglet:$(TAG) . -f odiglet/debug.Dockerfile
	kind load docker-image $(ORG)/odigos-odiglet:$(TAG)
	kubectl delete pod -n odigos-system -l app.kubernetes.io/name=odiglet
	kubectl wait --for=condition=ready pod -n odigos-system -l app.kubernetes.io/name=odiglet --timeout=180s
	kubectl port-forward -n odigos-system daemonset/odiglet 2345:2345

.PHONY: deploy
deploy: deploy-odiglet deploy-instrumentor

.PHONY: e2e-test
e2e-test:
	./e2e-test.sh

ALL_GO_MOD_DIRS := $(shell go list -m -f '{{.Dir}}' | sort)

.PHONY: go-mod-tidy
go-mod-tidy: $(ALL_GO_MOD_DIRS:%=go-mod-tidy/%)
go-mod-tidy/%: DIR=$*
go-mod-tidy/%:
	@cd $(DIR) && go mod tidy -compat=1.21

.PHONY: check-clean-work-tree
check-clean-work-tree:
	if [ -n "$$(git status --porcelain)" ]; then \
		git status; \
		git --no-pager diff; \
		echo 'Working tree is not clean, did you forget to run "make go-mod-tidy"?'; \
		exit 1; \
	fi