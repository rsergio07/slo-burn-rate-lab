# slo-burn-rate-lab

Turn an SLO into alerts that page for fast failures, open a ticket for slow ones, and clear quickly once the problem is gone.

**Video walkthrough:** on the [Reliability Bench](https://www.youtube.com/@reliabilitybench) YouTube channel

## What you will learn

- What an error budget is
- Why burn rate beats a plain error threshold
- How multiwindow alerts work
- How to test alerts by injecting real errors

**SLO:** 99.9% of checkouts complete. Windows are compressed for the demo: 2 minutes and 30 seconds stand in for 1 hour and 5 minutes.

## Requirements

Docker with 4 CPUs and 8 GiB of memory, plus `git`, `make`, `kind`, `kubectl`, `helm`, `jq` and `curl`. Open source only, and it runs locally. The first run downloads several images.

Tested on one setup: macOS arm64, Colima 0.10.3 (4 CPUs, 8 GiB, `vz`), kind 0.33.0 with Kubernetes 1.37, Helm 4.3.1 and kube-prometheus-stack 92.18.0. On Colima, first run:

```bash
colima ssh -- sudo sysctl -w fs.inotify.max_user_instances=1024 fs.inotify.max_user_watches=1048576
```

## The lab

Clone the repository, then run every command from its root.

```bash
git clone https://github.com/rsergio07/slo-burn-rate-lab.git slo-burn-rate-lab
cd slo-burn-rate-lab
```

### Step 1: Check your machine

Every line should say `ok`.

```bash
make preflight
```

### Step 2: Create the cluster

The private `.kubeconfig` keeps this lab away from your other clusters. Run the `export` again in any new terminal.

```bash
kind create cluster --config cluster/kind.yaml --kubeconfig .kubeconfig
export KUBECONFIG=$PWD/.kubeconfig
kubectl get nodes
```

### Step 3: Build the app and load it into the cluster

kind nodes have their own container runtime, so the image must be loaded into them.

```bash
docker build -t shop:lab app
kind load docker-image shop:lab --name lab
```

### Step 4: Install monitoring

This installs Prometheus and Grafana with only what the lab needs.

```bash
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update
helm upgrade --install monitoring prometheus-community/kube-prometheus-stack --version 92.18.0 -n monitoring --create-namespace -f deploy/monitoring-values.yaml --wait --timeout 8m
```

### Step 5: Deploy the shop

Grafana is at <http://localhost:30300/d/lab> with no login.

```bash
kubectl apply -f deploy/dashboard.yaml
kubectl apply -f deploy/shop.yaml
kubectl -n shop rollout status deployment/storefront --timeout=2m
kubectl -n shop get pods
```

### Step 6: Measure the baseline

Wait for traffic to build up.

```bash
# Wait 90 seconds
./scripts/verdict.sh 60
```

### Step 7: Add the SLO rules

`CheckoutFastBurn` pages at 14.4 times the allowed rate. `CheckoutSlowBurn` opens a ticket at 6 times. Both windows must agree.

```bash
cat deploy/slo-rules.yaml
kubectl apply -f deploy/slo-rules.yaml
# Wait 30 seconds
./scripts/burn.sh
```

### Step 8: Create a slow burn

Make 1% of checkouts fail. Wait about 2 minutes.

```bash
kubectl -n shop set env deployment/storefront ERROR_RATE=0.01
kubectl -n shop rollout status deployment/storefront --timeout=2m
# Wait 130 seconds
./scripts/burn.sh
```

### Step 9: Create a fast burn

Make 5% of checkouts fail. Wait about 45 seconds.

```bash
kubectl -n shop set env deployment/storefront ERROR_RATE=0.05
kubectl -n shop rollout status deployment/storefront --timeout=2m
# Wait 45 seconds
./scripts/burn.sh
```

### Step 10: Fix the problem

The short window lets alerts clear quickly, even while the 2 minute window is still high.

```bash
kubectl -n shop set env deployment/storefront ERROR_RATE=0
kubectl -n shop rollout status deployment/storefront --timeout=2m
# Wait 45 seconds
./scripts/burn.sh
# Wait 70 seconds
./scripts/burn.sh
```

### Step 11: Clean up

```bash
kind delete cluster --name lab --kubeconfig .kubeconfig
```

## Challenges

Predict the result before each run.

1. Change the SLO to 99.5% and recalculate both thresholds.
2. Remove the 30 second window. How long does the alert stay active?
3. Add an Alertmanager route that treats `page` and `ticket` differently.

## Safety

The lab never changes your default Kubernetes context. Grafana allows anonymous viewing and its ports are for local use. Do not expose them.

## License

MIT
