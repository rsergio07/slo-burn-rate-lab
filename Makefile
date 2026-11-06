export KUBECONFIG := $(CURDIR)/.kubeconfig

preflight:
	@./scripts/preflight.sh

down:
	@kind delete cluster --name lab --kubeconfig $(KUBECONFIG)
