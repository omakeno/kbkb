IMG_MANAGER ?= kbkb-manager:latest
IMG_SCHEDULER ?= kbkb-scheduler:latest
VERSION ?= v2.0.0
REGISTRY ?= ghcr.io/omakeno

.PHONY: all
all: generate manifests fmt vet test build

##@ Development

.PHONY: generate
generate: ## generate deepcopy code
	go tool controller-gen object paths=./api/...

.PHONY: manifests
manifests: ## generate CRD, RBAC and webhook manifests
	go tool controller-gen crd rbac:roleName=manager-role webhook paths=./... \
		output:crd:dir=config/crd output:rbac:dir=config/rbac output:webhook:dir=config/webhook

.PHONY: fmt
fmt:
	go fmt ./...

.PHONY: vet
vet:
	go vet ./...

.PHONY: test
test:
	go test ./...

##@ Build

.PHONY: build
build: ## build all binaries into bin/
	go build -o bin/manager ./cmd/manager
	go build -o bin/kbkb-scheduler ./cmd/scheduler
	go build -o bin/kubectl-kbkb ./cmd/kubectl-kbkb

.PHONY: docker-build
docker-build: ## build the manager and scheduler images
	docker build --build-arg CMD=manager -t $(IMG_MANAGER) .
	docker build --build-arg CMD=scheduler -t $(IMG_SCHEDULER) .

.PHONY: docker-release
docker-release: docker-build ## tag and push release images
	docker tag $(IMG_MANAGER) $(REGISTRY)/kbkb-manager:$(VERSION)
	docker tag $(IMG_SCHEDULER) $(REGISTRY)/kbkb-scheduler:$(VERSION)
	docker push $(REGISTRY)/kbkb-manager:$(VERSION)
	docker push $(REGISTRY)/kbkb-scheduler:$(VERSION)

.PHONY: installer
installer: manifests ## render the single-file installer
	go run sigs.k8s.io/kustomize/kustomize/v5@latest build config/release > install/kbkb.yaml

##@ Deployment

.PHONY: install
install: ## install the CRD
	kubectl apply -k config/crd

.PHONY: deploy
deploy: ## deploy everything (requires cert-manager)
	kubectl apply -k config/default

.PHONY: undeploy
undeploy:
	kubectl delete -k config/default

.PHONY: run-scheduler
run-scheduler: ## run the scheduler locally against the current kubeconfig
	go run ./cmd/scheduler --namespace=default --mode=manual

.PHONY: run-manager
run-manager: ## run the manager locally (webhook disabled: no certs)
	go run ./cmd/manager --enable-webhook=false
