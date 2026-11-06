#!/usr/bin/env bash
export KIND_VERSION="v0.33.0"
export CLUSTER="lab"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export ROOT
export KUBECONFIG="$ROOT/.kubeconfig"
