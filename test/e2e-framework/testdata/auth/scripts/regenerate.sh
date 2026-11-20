#!/usr/bin/env bash
# Regenerates the e2e auth-test fixtures. Run when creds/certs/chart change.
# Output files are committed to the repo so CI does not need to regenerate them.
#
# Requires: openssl, helm, and either htpasswd OR docker (with httpd:2-alpine).
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$HERE/.."

mkdir -p "$ROOT/certs"
mkdir -p "$ROOT/chart"

# 1. htpasswd (test-user:test-pass, bcrypt cost 5). Prefer local binary, fall
#    back to docker httpd:2-alpine when htpasswd is not installed.
if command -v htpasswd >/dev/null 2>&1; then
    htpasswd -B -b -c "$ROOT/htpasswd" test-user test-pass
else
    docker run --rm httpd:2-alpine htpasswd -Bbn test-user test-pass > "$ROOT/htpasswd"
fi

# 2. self-signed CA + server cert valid for the in-cluster service names
openssl req -x509 -newkey rsa:2048 -nodes -keyout "$ROOT/certs/ca.key" \
    -out "$ROOT/certs/ca.crt" -days 3650 -subj "/CN=kubevela-auth-test-ca"
openssl req -new -newkey rsa:2048 -nodes -keyout "$ROOT/certs/server.key" \
    -out "$ROOT/certs/server.csr" \
    -subj "/CN=*.kubevela-auth-test.svc.cluster.local" \
    -addext "subjectAltName=DNS:zot.kubevela-auth-test.svc.cluster.local,DNS:chartmuseum.kubevela-auth-test.svc.cluster.local,DNS:chartmuseum-bearer.kubevela-auth-test.svc.cluster.local"
openssl x509 -req -in "$ROOT/certs/server.csr" -CA "$ROOT/certs/ca.crt" -CAkey "$ROOT/certs/ca.key" \
    -CAcreateserial -out "$ROOT/certs/server.crt" -days 365 \
    -extfile <(printf "subjectAltName=DNS:zot.kubevela-auth-test.svc.cluster.local,DNS:chartmuseum.kubevela-auth-test.svc.cluster.local,DNS:chartmuseum-bearer.kubevela-auth-test.svc.cluster.local")
rm -f "$ROOT/certs/server.csr" "$ROOT/certs/ca.srl" "$ROOT/certs/ca.key"

# 3. chart tarball
helm package "$ROOT/chart/source" -d "$ROOT/chart" --version 1.0.0
mv "$ROOT/chart/podinfo-1.0.0.tgz" "$ROOT/chart/podinfo-test-1.0.0.tgz"
