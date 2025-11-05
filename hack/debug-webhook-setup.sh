#!/bin/bash
# Webhook debugging setup script for KubeVela
# This script sets up everything needed to debug webhooks locally

set -e

# Configuration
CERT_DIR="k8s-webhook-server/serving-certs"
NAMESPACE="vela-system"
SECRET_NAME="webhook-server-cert"
WEBHOOK_CONFIG_NAME="kubevela-vela-core-admission"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}=== KubeVela Webhook Debug Setup ===${NC}"

# Function to check prerequisites
check_prerequisites() {
    echo -e "${YELLOW}Checking prerequisites...${NC}"

    # Check kubectl
    if ! command -v kubectl &> /dev/null; then
        echo -e "${RED}kubectl is not installed${NC}"
        exit 1
    fi

    # Check openssl
    if ! command -v openssl &> /dev/null; then
        echo -e "${RED}openssl is not installed${NC}"
        exit 1
    fi

    # Check cluster connectivity
    if ! kubectl cluster-info &> /dev/null; then
        echo -e "${RED}Cannot connect to Kubernetes cluster${NC}"
        echo "Please ensure your kubeconfig is set up correctly"
        exit 1
    fi

    # Wait for cluster to be ready
    echo "Waiting for cluster nodes to be ready..."
    kubectl wait --for=condition=Ready nodes --all --timeout=60s &> /dev/null || true

    echo -e "${GREEN}Prerequisites check passed${NC}"
}

# Function to create namespace if not exists
create_namespace() {
    echo -e "${YELLOW}Creating namespace ${NAMESPACE}...${NC}"
    kubectl create namespace ${NAMESPACE} --dry-run=client -o yaml | kubectl apply -f -
    echo -e "${GREEN}Namespace ready${NC}"
}

# Function to generate certificates
generate_certificates() {
    echo -e "${YELLOW}Generating webhook certificates...${NC}"

    # Create directory
    mkdir -p ${CERT_DIR}

    # Clean old certificates
    rm -f ${CERT_DIR}/*

    # Generate CA private key
    openssl genrsa -out ${CERT_DIR}/ca.key 2048

    # Generate CA certificate
    openssl req -x509 -new -nodes -key ${CERT_DIR}/ca.key -days 365 -out ${CERT_DIR}/ca.crt \
        -subj "/CN=webhook-ca"

    # Generate server private key
    openssl genrsa -out ${CERT_DIR}/tls.key 2048

    # Get host IP for Docker internal network
    # NOTE: 192.168.5.2 is the standard k3d host gateway IP that allows containers to reach the host machine
    # This is only for local k3d development environments - DO NOT use this script in production
    # With failurePolicy: Fail, an unreachable webhook can block CRD operations cluster-wide
    HOST_IP="192.168.5.2"
    LOCAL_IP=$(ifconfig | grep "inet " | grep -v 127.0.0.1 | head -1 | awk '{print $2}')

    # Create certificate config with SANs
    cat > /tmp/webhook.conf << EOF
[req]
req_extensions = v3_req
distinguished_name = req_distinguished_name
[req_distinguished_name]
[v3_req]
basicConstraints = CA:FALSE
keyUsage = nonRepudiation, digitalSignature, keyEncipherment
subjectAltName = @alt_names
[alt_names]
DNS.1 = localhost
DNS.2 = vela-webhook.${NAMESPACE}.svc
DNS.3 = vela-webhook.${NAMESPACE}.svc.cluster.local
DNS.4 = *.${NAMESPACE}.svc
DNS.5 = *.${NAMESPACE}.svc.cluster.local
IP.1 = 127.0.0.1
IP.2 = ${HOST_IP}
IP.3 = ${LOCAL_IP}
EOF

    # Generate certificate request
    openssl req -new -key ${CERT_DIR}/tls.key -out /tmp/server.csr \
        -subj "/CN=vela-webhook.${NAMESPACE}.svc" -config /tmp/webhook.conf

    # Generate server certificate with SANs
    openssl x509 -req -in /tmp/server.csr -CA ${CERT_DIR}/ca.crt -CAkey ${CERT_DIR}/ca.key \
        -CAcreateserial -out ${CERT_DIR}/tls.crt -days 365 \
        -extensions v3_req -extfile /tmp/webhook.conf

    echo -e "${GREEN}Certificates generated with IP SANs: 127.0.0.1, ${HOST_IP}, ${LOCAL_IP}${NC}"

    # Clean up temp files
    rm -f /tmp/server.csr /tmp/webhook.conf
}

# Function to create Kubernetes secret
create_k8s_secret() {
    echo -e "${YELLOW}Creating Kubernetes secret...${NC}"

    # Delete old secret if exists
    kubectl delete secret ${SECRET_NAME} -n ${NAMESPACE} --ignore-not-found

    # Create new secret
    kubectl create secret tls ${SECRET_NAME} \
        --cert=${CERT_DIR}/tls.crt \
        --key=${CERT_DIR}/tls.key \
        -n ${NAMESPACE}

    echo -e "${GREEN}Secret ${SECRET_NAME} created in namespace ${NAMESPACE}${NC}"
}

# Function to create webhook configuration
create_webhook_config() {
    echo -e "${YELLOW}Creating webhook configuration...${NC}"

    # Get CA bundle
    CA_BUNDLE=$(cat ${CERT_DIR}/ca.crt | base64 | tr -d '\n')

    # Delete old webhook configuration if exists
    kubectl delete validatingwebhookconfiguration ${WEBHOOK_CONFIG_NAME} --ignore-not-found

    # Create webhook configuration
    cat > /tmp/webhook-config.yaml << EOF
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  name: ${WEBHOOK_CONFIG_NAME}
webhooks:
- name: componentdefinition.core.oam.dev
  clientConfig:
    url: https://${HOST_IP}:9445/validating-core-oam-dev-v1beta1-componentdefinitions
    caBundle: ${CA_BUNDLE}
  rules:
  - apiGroups: ["core.oam.dev"]
    apiVersions: ["v1beta1"]
    resources: ["componentdefinitions"]
    operations: ["CREATE", "UPDATE"]
  admissionReviewVersions: ["v1", "v1beta1"]
  sideEffects: None
  failurePolicy: Fail
- name: traitdefinition.core.oam.dev
  clientConfig:
    url: https://${HOST_IP}:9445/validating-core-oam-dev-v1beta1-traitdefinitions
    caBundle: ${CA_BUNDLE}
  rules:
  - apiGroups: ["core.oam.dev"]
    apiVersions: ["v1beta1"]
    resources: ["traitdefinitions"]
    operations: ["CREATE", "UPDATE"]
  admissionReviewVersions: ["v1", "v1beta1"]
  sideEffects: None
  failurePolicy: Fail
- name: policydefinition.core.oam.dev
  clientConfig:
    url: https://${HOST_IP}:9445/validating-core-oam-dev-v1beta1-policydefinitions
    caBundle: ${CA_BUNDLE}
  rules:
  - apiGroups: ["core.oam.dev"]
    apiVersions: ["v1beta1"]
    resources: ["policydefinitions"]
    operations: ["CREATE", "UPDATE"]
  admissionReviewVersions: ["v1", "v1beta1"]
  sideEffects: None
  failurePolicy: Fail
- name: workflowstepdefinition.core.oam.dev
  clientConfig:
    url: https://${HOST_IP}:9445/validating-core-oam-dev-v1beta1-workflowstepdefinitions
    caBundle: ${CA_BUNDLE}
  rules:
  - apiGroups: ["core.oam.dev"]
    apiVersions: ["v1beta1"]
    resources: ["workflowstepdefinitions"]
    operations: ["CREATE", "UPDATE"]
  admissionReviewVersions: ["v1", "v1beta1"]
  sideEffects: None
  failurePolicy: Fail
- name: applications.core.oam.dev
  clientConfig:
    url: https://${HOST_IP}:9445/validating-core-oam-dev-v1beta1-applications
    caBundle: ${CA_BUNDLE}
  rules:
  - apiGroups: ["core.oam.dev"]
    apiVersions: ["v1beta1"]
    resources: ["applications"]
    operations: ["CREATE", "UPDATE"]
  admissionReviewVersions: ["v1", "v1beta1"]
  sideEffects: None
  failurePolicy: Fail
EOF

    kubectl apply -f /tmp/webhook-config.yaml
    rm -f /tmp/webhook-config.yaml

    echo -e "${GREEN}Webhook configuration created${NC}"
}

# Function to show next steps
show_next_steps() {
    echo -e "${GREEN}"
    echo "========================================="
    echo "Webhook debugging setup complete!"
    echo "========================================="
    echo -e "${NC}"
    echo "Next steps:"
    echo "1. Open VS Code"
    echo "2. Set breakpoints in webhook validation code:"
    echo "   - pkg/webhook/utils/utils.go:141"
    echo "   - pkg/webhook/core.oam.dev/v1beta1/componentdefinition/validating_handler.go:74"
    echo "3. Press F5 and select 'Debug Webhook Validation'"
    echo "4. Wait for webhook server to start (port 9445)"
    echo "5. Test with kubectl apply commands"
    echo ""
    echo -e "${YELLOW}Test command (should be rejected):${NC}"
    echo 'kubectl apply -f test/webhook-test-invalid.yaml'
    echo ""
    echo -e "${GREEN}The webhook will reject ComponentDefinitions with non-existent CRDs${NC}"
}

# Main execution
main() {
    check_prerequisites
    create_namespace
    generate_certificates
    create_k8s_secret
    create_webhook_config
    show_next_steps
}

# Run main function
main "$@"