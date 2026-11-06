#!/usr/bin/env bash
# Checks everything the lab needs. Exit code is the number of failed checks.
set -u
source "$(dirname "$0")/lib.sh"
fails=0
pass() { printf '  ok    %s\n' "$1"; }
fail() { printf '  FAIL  %s\n' "$1"; fails=$((fails + 1)); }

for tool in docker kind kubectl helm jq; do
  command -v "$tool" >/dev/null && pass "$tool found" || fail "$tool is not installed"
done

if docker info >/dev/null 2>&1; then
  pass "docker daemon reachable"
  cpus=$(docker info --format '{{.NCPU}}')
  mem_gib=$(($(docker info --format '{{.MemTotal}}') / 1024 / 1024 / 1024))
  [ "$cpus" -ge 4 ] && pass "docker has $cpus CPUs" || fail "docker has $cpus CPUs, needs 4"
  [ "$mem_gib" -ge 7 ] && pass "docker has ${mem_gib} GiB" || fail "docker has ${mem_gib} GiB, needs 8"
else
  fail "docker daemon not reachable"
fi

command -v kind >/dev/null && [ "$(kind version | awk '{print $2}')" != "$KIND_VERSION" ] &&
  printf '  note  tested with kind %s, you have %s\n' "$KIND_VERSION" "$(kind version | awk '{print $2}')"
exit "$fails"
