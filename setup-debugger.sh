#!/usr/bin/env bash

set -euo pipefail

#===============================================================================
# Script: setup-kubevela-debugger.sh
# Purpose: Install and configure the KubeVela controller in debug mode
#          with webhooks, using the provided IP and port, generate certs,
#          update code options, and deploy the webhook secret and configurations.
# Usage:   ./setup-kubevela-debugger.sh <IP_ADDRESS> <PORT>
# Example: ./setup-kubevela-debugger.sh 192.168.1.100 9090
#
# NOTE: This script must be run from the root of the kubevela repository!
#===============================================================================

#--- Helper: show usage and exit -----------------------------------------------
usage() {
  cat <<EOF
Usage: $0 <IP_ADDRESS> <PORT>

Installs/configures the KubeVela controller in debug mode with webhooks.
  <IP_ADDRESS>   IP the controller will bind to (e.g. 10.0.01)
  <PORT>         Port the controller will listen on (e.g. 9443)

Example:
  $0 192.168.1.100 9090
EOF
  exit 1
}

#--- Parse and validate args --------------------------------------------------
if [[ $# -ne 2 ]]; then
  echo "ERROR: Wrong number of arguments."
  usage
fi
IP_ADDR="$1"
PORT="$2"

# Validate IP
if ! [[ $IP_ADDR =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "ERROR: '$IP_ADDR' is not a valid IPv4 address."
  usage
fi
# Validate Port
if ! [[ $PORT =~ ^[0-9]+$ ]] || (( PORT < 1 || PORT > 65535 )); then
  echo "ERROR: '$PORT' is not a valid port number."
  usage
fi

echo "➔ Using IP:   ${IP_ADDR}"
echo "➔ Using Port: ${PORT}"
echo

#--- Ensure script is run from kubevela repo root ------------------------------
REPO_DIR_NAME="$(basename "$(pwd)")"
if [[ "$REPO_DIR_NAME" != "kubevela" ]]; then
  echo "ERROR: Script must run from the root of the kubevela repository."
  exit 1
fi

echo "==> Running in kubevela repo root"

#--- STEP 0: Prepare directory ------------------------------------------------
echo "==> STEP 0: Create serving certificates directory"
mkdir -p k8s-webhook-server/serving-certs

echo "==> Directory ready: k8s-webhook-server/serving-certs"

#--- STEP 1: Generate CA ----------------------------------------------------
echo "==> STEP 1: Generate CA private key and self-signed cert"
pushd k8s-webhook-server/serving-certs > /dev/null
openssl genrsa -out ca.key 2048
openssl req -x509 -new -nodes -key ca.key -days 3650 -out ca.crt -subj "/CN=Webhook CA"
popd > /dev/null

echo "==> CA key and cert generated"

#--- STEP 2: OpenSSL config -------------------------------------------------
echo "==> STEP 2: Create openssl-webhook.cnf"
pushd k8s-webhook-server/serving-certs > /dev/null
cat <<EOF > openssl-webhook.cnf
[ req ]
default_bits       = 2048
prompt             = no
default_md         = sha256
distinguished_name = dn
req_extensions     = req_ext

[ dn ]
CN = ${IP_ADDR}

[ req_ext ]
subjectAltName = @alt_names

[ alt_names ]
IP.1 = ${IP_ADDR}
EOF
popd > /dev/null

echo "==> OpenSSL config created"

#--- STEP 3: Webhook certs --------------------------------------------------
echo "==> STEP 3: Generate TLS key, CSR and signed cert"
pushd k8s-webhook-server/serving-certs > /dev/null
openssl genrsa -out tls.key 2048
openssl req -new -key tls.key -out webhook.csr -config openssl-webhook.cnf
openssl x509 -req -in webhook.csr -CA ca.crt -CAkey ca.key -CAcreateserial \
  -out tls.crt -days 365 -extensions req_ext -extfile openssl-webhook.cnf
popd > /dev/null

echo "==> TLS key, CSR, and cert generated"

#--- STEP 4: Update CoreOptions ---------------------------------------------
echo "==> STEP 4: Enable webhook in CoreOptions"
OPTIONS_FILE="cmd/core/app/options/options.go"
[[ -f "$OPTIONS_FILE" ]] || { echo "ERROR: $OPTIONS_FILE not found"; exit 1; }
cp "$OPTIONS_FILE" "${OPTIONS_FILE}.bak"
# macOS sed - in-place
sed -E -i '' 's/(UseWebhook:[[:space:]]*)false/\1true/' "$OPTIONS_FILE"
sed -E -i '' "s|(CertDir:[[:space:]]*)\"[^\"]*\"|\1\"$(pwd)/k8s-webhook-server/serving-certs\"|" "$OPTIONS_FILE"
sed -E -i '' "s/(WebhookPort:[[:space:]]*)[0-9]+/\1${PORT}/" "$OPTIONS_FILE"
echo "==> CoreOptions updated"

#--- STEP 5: Wait for debugger -----------------------------------------------
echo "==> STEP 5: Start controller in debug mode"
read -rp "Press [ENTER] once the controller is running in debug mode... "

echo "==> Continuing after debug start"

#--- STEP 6: Export KUBECONFIG ----------------------------------------------
echo "==> STEP 6: Export KUBECONFIG"
export KUBECONFIG="${HOME}/.kube/config"
echo "Using KUBECONFIG=${KUBECONFIG}"

#--- STEP 7: Encode certs --------------------------------------------------
echo "==> STEP 7: Encode certificates to Base64"
pushd k8s-webhook-server/serving-certs > /dev/null
CA_CRT_B64=$(base64 -i ca.crt | tr -d '\n')
TLS_CRT_B64=$(base64 -i tls.crt | tr -d '\n')
TLS_KEY_B64=$(base64 -i tls.key | tr -d '\n')
popd > /dev/null

echo "==> Certificates encoded"

#--- STEP 8: Create and apply Secret ----------------------------------------
echo "==> STEP 8: Create webhook secret"
SECRET_YAML="k8s-webhook-server/serving-certs/webhook-secret.yaml"
cat <<EOF > "$SECRET_YAML"
apiVersion: v1
kind: Secret
metadata:
  name: kubevela-vela-core-admission
  namespace: vela-system
type: Opaque
data:
  ca.crt: ${CA_CRT_B64}
  tls.crt: ${TLS_CRT_B64}
  tls.key: ${TLS_KEY_B64}
EOF
kubectl apply -f "$SECRET_YAML"
echo "==> Secret applied"

#--- Create webhook configuration manifests -----------------------------------
echo "==> STEP 9: Create webhook configuration manifest files"
pushd k8s-webhook-server/serving-certs >/dev/null
cat <<EOF >MutatingWebhookConfiguration.yaml
apiVersion: admissionregistration.k8s.io/v1
kind: MutatingWebhookConfiguration
metadata:
  annotations:
    meta.helm.sh/release-name: kubevela
    meta.helm.sh/release-namespace: vela-system
  labels:
    app.kubernetes.io/managed-by: Helm
  name: kubevela-vela-core-admission
webhooks:
- admissionReviewVersions:
  - v1beta1
  - v1
  clientConfig:
    caBundle: ${CA_CRT_B64}
    url: "https://${IP_ADDR}:${PORT}/mutating-core-oam-dev-v1beta1-applications"
  failurePolicy: Fail
  matchPolicy: Equivalent
  name: mutating.core.oam.dev.v1beta1.applications
  namespaceSelector: {}
  objectSelector: {}
  reinvocationPolicy: Never
  rules:
  - apiGroups:
      - core.oam.dev
    apiVersions:
      - v1beta1
    operations:
      - CREATE
      - UPDATE
    resources:
      - applications
    scope: '*'
  sideEffects: None
  timeoutSeconds: 10
- admissionReviewVersions:
  - v1beta1
  - v1
  clientConfig:
    caBundle: ${CA_CRT_B64}
    url: "https://${IP_ADDR}:${PORT}/mutating-core-oam-dev-v1beta1-componentdefinitions"
  failurePolicy: Fail
  matchPolicy: Equivalent
  name: mutating.core.oam.dev.v1beta1.componentdefinitions
  namespaceSelector: {}
  objectSelector: {}
  reinvocationPolicy: Never
  rules:
  - apiGroups:
      - core.oam.dev
    apiVersions:
      - v1beta1
    operations:
      - CREATE
      - UPDATE
    resources:
      - componentdefinitions
    scope: '*'
  sideEffects: None
  timeoutSeconds: 10
EOF
cat <<EOF >ValidatingWebhookConfiguration.yaml
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  annotations:
    meta.helm.sh/release-name: kubevela
    meta.helm.sh/release-namespace: vela-system
  name: kubevela-vela-core-admission
webhooks:
- admissionReviewVersions:
  - v1beta1
  - v1
  clientConfig:
    caBundle: ${CA_CRT_B64}
    url: "https://${IP_ADDR}:${PORT}/validating-core-oam-dev-v1beta1-traitdefinitions"
  failurePolicy: Fail
  matchPolicy: Equivalent
  name: validating.core.oam.dev.v1beta1.traitdefinitions
  namespaceSelector: {}
  objectSelector: {}
  rules:
  - apiGroups:
      - core.oam.dev
    apiVersions:
      - v1beta1
    operations:
      - CREATE
      - UPDATE
    resources:
      - traitdefinitions
    scope: '*'
  sideEffects: None
  timeoutSeconds: 5
- admissionReviewVersions:
  - v1beta1
  - v1
  clientConfig:
    caBundle: ${CA_CRT_B64}
    url: "https://${IP_ADDR}:${PORT}/validating-core-oam-dev-v1beta1-applications"
  failurePolicy: Fail
  matchPolicy: Equivalent
  name: validating.core.oam.dev.v1beta1.applications
  namespaceSelector: {}
  objectSelector: {}
  rules:
  - apiGroups:
      - core.oam.dev
    apiVersions:
      - v1beta1
    operations:
      - CREATE
      - UPDATE
    resources:
      - applications
    scope: '*'
  sideEffects: None
  timeoutSeconds: 10
- admissionReviewVersions:
  - v1beta1
  - v1
  clientConfig:
    caBundle: ${CA_CRT_B64}
    url: "https://${IP_ADDR}:${PORT}/validating-core-oam-dev-v1beta1-componentdefinitions"
  failurePolicy: Fail
  matchPolicy: Equivalent
  name: validating.core.oam.dev.v1beta1.componentdefinitions
  namespaceSelector: {}
  objectSelector: {}
  rules:
  - apiGroups:
      - core.oam.dev
    apiVersions:
      - v1beta1
    operations:
      - CREATE
      - UPDATE
    resources:
      - componentdefinitions
    scope: '*'
  sideEffects: None
  timeoutSeconds: 10
- admissionReviewVersions:
  - v1beta1
  - v1
  clientConfig:
    caBundle: ${CA_CRT_B64}
    url: "https://${IP_ADDR}:${PORT}/validating-core-oam-dev-v1beta1-policydefinitions"
  failurePolicy: Fail
  matchPolicy: Equivalent
  name: validating.core.oam.dev.v1beta1.policydefinitions
  namespaceSelector: {}
  objectSelector: {}
  rules:
  - apiGroups:
      - core.oam.dev
    apiVersions:
      - v1beta1
    operations:
      - CREATE
      - UPDATE
    resources:
      - policydefinitions
    scope: '*'
  sideEffects: None
  timeoutSeconds: 10
EOF
popd >/dev/null

#--- Apply webhook configurations ---------------------------------------------
echo "==> STEP 10: Apply webhook configuration manifests"
kubectl apply -f k8s-webhook-server/serving-certs/ValidatingWebhookConfiguration.yaml
kubectl apply -f k8s-webhook-server/serving-certs/MutatingWebhookConfiguration.yaml

echo "🎉 Setup complete!"
exit 0