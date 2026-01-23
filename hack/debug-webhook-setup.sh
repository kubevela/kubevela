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

    # Auto-detect host IP for Docker/k3d internal network
    # This is only for local k3d development environments - DO NOT use this script in production
    # With failurePolicy: Fail, an unreachable webhook can block CRD operations cluster-wide

    # Try to detect k3d cluster
    K3D_CLUSTER=$(kubectl config current-context | grep -o 'k3d-[^@]*' | sed 's/k3d-//' || echo "")

    if [ -n "$K3D_CLUSTER" ]; then
        echo "Detected k3d cluster: $K3D_CLUSTER"

        # Check if k3d is using host network
        NETWORK_MODE=$(docker inspect "k3d-${K3D_CLUSTER}-server-0" 2>/dev/null | grep -o '"NetworkMode": "[^"]*"' | cut -d'"' -f4 || echo "")

        if [ "$NETWORK_MODE" = "host" ]; then
            # Host network mode - detect OS
            if [ "$(uname)" = "Darwin" ]; then
                # macOS with Docker Desktop - use host.docker.internal
                echo "Detected k3d with --network host on macOS, using host.docker.internal"
                HOST_IP="host.docker.internal"
            else
                # Linux - true host networking works
                echo "Detected k3d with --network host, using localhost"
                HOST_IP="127.0.0.1"
            fi
        else
            # Bridge network mode - get gateway IP
            NETWORK_NAME="k3d-${K3D_CLUSTER}"
            HOST_IP=$(docker network inspect "$NETWORK_NAME" -f '{{range .IPAM.Config}}{{.Gateway}}{{end}}' 2>/dev/null || echo "")

            if [ -z "$HOST_IP" ]; then
                # Fallback to common k3d gateway IPs
                echo "Could not detect gateway IP, trying common defaults..."
                if docker exec "k3d-${K3D_CLUSTER}-server-0" getent hosts host.k3d.internal 2>/dev/null | awk '{print $1}' | grep -q .; then
                    HOST_IP=$(docker exec "k3d-${K3D_CLUSTER}-server-0" cat /etc/hosts | grep host.k3d.internal | awk '{print $1}')
                else
                    HOST_IP="172.18.0.1"
                fi
            fi

            echo "Detected k3d with bridge network, using gateway IP: $HOST_IP"
        fi
    else
        # Not k3d, use default
        echo "Not using k3d, defaulting to 192.168.5.2"
        HOST_IP="192.168.5.2"
    fi

    # Get local machine IP for SANs (optional, for reference)
    if command -v ifconfig &> /dev/null; then
        LOCAL_IP=$(ifconfig | grep "inet " | grep -v 127.0.0.1 | head -1 | awk '{print $2}')
    elif command -v ip &> /dev/null; then
        LOCAL_IP=$(ip -4 addr show | grep -oP '(?<=inet\s)\d+(\.\d+){3}' | grep -v 127.0.0.1 | head -1)
    else
        LOCAL_IP=""
    fi

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
DNS.6 = host.k3d.internal
DNS.7 = host.docker.internal
DNS.8 = host.lima.internal
IP.1 = 127.0.0.1
EOF

    # Add HOST_IP - check if it's a hostname or IP
    if [[ "$HOST_IP" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
        # It's an IP address
        echo "IP.2 = ${HOST_IP}" >> /tmp/webhook.conf
    else
        # It's a hostname - already covered by DNS SANs above
        echo "# HOST_IP is hostname: ${HOST_IP} (already in DNS SANs)" >> /tmp/webhook.conf
    fi

    # Add LOCAL_IP to SANs only if detected and is an IP
    if [ -n "$LOCAL_IP" ] && [[ "$LOCAL_IP" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
        echo "IP.3 = ${LOCAL_IP}" >> /tmp/webhook.conf
    fi

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

    echo "Configuration:"
    echo "  - Webhook URL: https://${HOST_IP}:9445"
    echo "  - Certificate directory: ${CERT_DIR}"

    if [ -n "$K3D_CLUSTER" ]; then
        echo "  - k3d cluster: ${K3D_CLUSTER}"
        if [ "$NETWORK_MODE" = "host" ]; then
            echo "  - Network mode: host (using ${HOST_IP})"
        else
            echo "  - Network mode: bridge (using gateway ${HOST_IP})"
        fi
    fi

    echo ""
    echo "Next steps:"
    echo "1. Open your IDE (VS Code, GoLand, etc.)"
    echo "2. Set breakpoints in webhook validation code:"
    echo "   - pkg/webhook/core.oam.dev/v1beta1/application/validating_handler.go:66"
    echo "   - pkg/webhook/core.oam.dev/v1beta1/componentdefinition/component_definition_validating_handler.go:74"
    echo "3. Start debugging cmd/core/main.go with arguments:"
    echo "   --use-webhook=true"
    echo "   --webhook-port=9445"
    echo "   --webhook-cert-dir=${CERT_DIR}"
    echo "   --leader-elect=false"
    echo "4. Wait for webhook server to start"
    echo "5. Test with kubectl apply commands"
    echo ""
    echo -e "${YELLOW}Test command:${NC}"
    echo 'kubectl apply -f <your-application.yaml>'
    echo ""
    echo -e "${GREEN}Your breakpoints will hit when kubectl applies resources!${NC}"
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