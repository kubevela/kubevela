#!/usr/bin/env bash

# Implemented based on Dapr Cli https://github.com/dapr/cli/tree/master/install

# Vela CLI location
: ${VELA_INSTALL_DIR:="/usr/local/bin"}

# sudo is required to copy binary to VELA_INSTALL_DIR for linux
: ${USE_SUDO:="false"}

# Http request CLI
VELA_HTTP_REQUEST_CLI=curl

# GitHub Organization and repo name to download release
GITHUB_ORG=oam-dev
GITHUB_REPO=kubevela

# Vela CLI filename
VELA_CLI_FILENAME=vela

VELA_CLI_FILE="${VELA_INSTALL_DIR}/${VELA_CLI_FILENAME}"

getSystemInfo() {
    ARCH=$(uname -m)
    case $ARCH in
        armv7*) ARCH="arm";;
        aarch64) ARCH="arm64";;
        x86_64) ARCH="amd64";;
    esac

    OS=$(echo `uname`|tr '[:upper:]' '[:lower:]')

    # Most linux distro needs root permission to copy the file to /usr/local/bin
    if [ "$OS" == "linux" ] && [ "$VELA_INSTALL_DIR" == "/usr/local/bin" ]; then
        USE_SUDO="true"
    fi
}

verifySupported() {
    local supported=(darwin-amd64 linux-amd64 linux-arm linux-arm64)
    local current_osarch="${OS}-${ARCH}"

    for osarch in "${supported[@]}"; do
        if [ "$osarch" == "$current_osarch" ]; then
            echo "Your system is ${OS}_${ARCH}"
            return
        fi
    done

    echo "No prebuilt binary for ${current_osarch}"
    exit 1
}

runAsRoot() {
    local CMD="$*"

    if [ $EUID -ne 0 -a $USE_SUDO = "true" ]; then
        CMD="sudo $CMD"
    fi

    $CMD
}

checkHttpRequestCLI() {
    if type "curl" > /dev/null; then
        VELA_HTTP_REQUEST_CLI=curl
    elif type "wget" > /dev/null; then
        VELA_HTTP_REQUEST_CLI=wget
    else
        echo "Either curl or wget is required"
        exit 1
    fi
}

checkExistingVela() {
    if [ -f "$VELA_CLI_FILE" ]; then
        echo -e "\nVela CLI is detected:"
        $VELA_CLI_FILE --version
        echo -e "Reinstalling Vela CLI - ${VELA_CLI_FILE}...\n"
    else
        echo -e "Installing Vela CLI...\n"
    fi
}

getLatestRelease() {
    local velaReleaseUrl="https://api.github.com/repos/${GITHUB_ORG}/${GITHUB_REPO}/releases"
    local latest_release=""

    if [ "$VELA_HTTP_REQUEST_CLI" == "curl" ]; then
        latest_release=$(curl -s $velaReleaseUrl | grep \"tag_name\" | grep -v rc | awk 'NR==1{print $2}' |  sed -n 's/\"\(.*\)\",/\1/p')
    else
        latest_release=$(wget -q --header="Accept: application/json" -O - $velaReleaseUrl | grep \"tag_name\" | grep -v rc | awk 'NR==1{print $2}' |  sed -n 's/\"\(.*\)\",/\1/p')
    fi

    ret_val=$latest_release
}

downloadFile() {
    LATEST_RELEASE_TAG=$1

    VELA_CLI_ARTIFACT="${VELA_CLI_FILENAME}-${LATEST_RELEASE_TAG}-${OS}-${ARCH}.tar.gz"
    # convert `-` to `_` to let it work
    DOWNLOAD_BASE="https://github.com/${GITHUB_ORG}/${GITHUB_REPO}/releases/download"
    DOWNLOAD_URL="${DOWNLOAD_BASE}/${LATEST_RELEASE_TAG}/${VELA_CLI_ARTIFACT}"

    # Create the temp directory
    VELA_TMP_ROOT=$(mktemp -dt vela-install-XXXXXX)
    ARTIFACT_TMP_FILE="$VELA_TMP_ROOT/$VELA_CLI_ARTIFACT"

    echo "Downloading $DOWNLOAD_URL ..."
    if [ "$VELA_HTTP_REQUEST_CLI" == "curl" ]; then
        curl -SsL "$DOWNLOAD_URL" -o "$ARTIFACT_TMP_FILE"
    else
        wget -q -O "$ARTIFACT_TMP_FILE" "$DOWNLOAD_URL"
    fi

    if [ ! -f "$ARTIFACT_TMP_FILE" ]; then
        echo "failed to download $DOWNLOAD_URL ..."
        exit 1
    fi
}

installFile() {
    tar xf "$ARTIFACT_TMP_FILE" -C "$VELA_TMP_ROOT"
    local tmp_root_vela_cli="$VELA_TMP_ROOT/${OS}-${ARCH}/$VELA_CLI_FILENAME"

    if [ ! -f "$tmp_root_vela_cli" ]; then
        echo "Failed to unpack Vela CLI executable."
        exit 1
    fi

    chmod o+x $tmp_root_vela_cli
    runAsRoot cp "$tmp_root_vela_cli" "$VELA_INSTALL_DIR"

    if [ -f "$VELA_CLI_FILE" ]; then
        echo "$VELA_CLI_FILENAME installed into $VELA_INSTALL_DIR successfully."

        $VELA_CLI_FILE --version
    else 
        echo "Failed to install $VELA_CLI_FILENAME"
        exit 1
    fi
}

fail_trap() {
    result=$?
    if [ "$result" != "0" ]; then
        echo "Failed to install Vela CLI"
        echo "For support, go to https://kubevela.io"
    fi
    cleanup
    exit $result
}

cleanup() {
    if [[ -d "${VELA_TMP_ROOT:-}" ]]; then
        rm -rf "$VELA_TMP_ROOT"
    fi
}

installCompleted() {
    echo -e "\nTo get started with KubeVela, please visit https://kubevela.io"
}

# -----------------------------------------------------------------------------
# main
# -----------------------------------------------------------------------------
trap "fail_trap" EXIT

getSystemInfo
verifySupported
checkExistingVela
checkHttpRequestCLI


if [ -z "$1" ]; then
    echo "Getting the latest Vela CLI..."
    getLatestRelease
else
    ret_val=v$1
fi

echo "Installing $ret_val Vela CLI..."

downloadFile $ret_val
installFile
cleanup

installCompleted
