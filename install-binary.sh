#!/bin/bash

# Combination of the Glide and Helm scripts, with my own tweaks.

PROJECT_NAME="irr"
PROJECT_GH="lucas-albers-lz4/$PROJECT_NAME"

# Get plugin path from helm env if HELM_HOME not set
if [ -z "$HELM_HOME" ]; then
  HELM_PLUGINS=$(helm env | grep HELM_PLUGINS | cut -d'"' -f2)
  if [ -n "$HELM_PLUGINS" ]; then
    HELM_PLUGIN_PATH="$HELM_PLUGINS/irr"
  else
    HELM_PLUGIN_PATH="$(helm home)/plugins/irr"
  fi
else
  HELM_PLUGIN_PATH="$HELM_HOME/plugins/irr"
fi

# Extract version from plugin.yaml if in the same directory
if [ -f "plugin.yaml" ]; then
  HELM_PLUGIN_VERSION=$(grep "version" plugin.yaml | cut -d'"' -f2)
fi

if [[ $SKIP_BIN_INSTALL == "1" ]]; then
  echo "Skipping binary install"
  exit
fi

# initArch discovers the architecture for this system.
initArch() {
  ARCH=$(uname -m)
  case $ARCH in
    armv5*) ARCH="armv5";;
    armv6*) ARCH="armv6";;
    armv7*) ARCH="armv7";;
    aarch64) ARCH="arm64";;
    x86) ARCH="386";;
    x86_64) ARCH="amd64";;
    i686) ARCH="386";;
    i386) ARCH="386";;
  esac
}

# initOS discovers the operating system for this system.
initOS() {
  OS=$(echo `uname`|tr '[:upper:]' '[:lower:]')

  case "$OS" in
    # Minimalist GNU for Windows
    mingw*) OS='windows';;
  esac
}

# verifySupported checks that the os/arch combination is supported for
# binary builds.
verifySupported() {
  local supported="linux-amd64\nlinux-arm64\ndarwin-amd64\ndarwin-arm64"
  if ! echo "${supported}" | grep -q "${OS}-${ARCH}"; then
    echo "No prebuild binary for ${OS}-${ARCH}."
    exit 1
  fi

  if ! type "curl" > /dev/null && ! type "wget" > /dev/null; then
    echo "Either curl or wget is required"
    exit 1
  fi
}

# getDownloadURL checks the latest available version.
getDownloadURL() {
  # If there's a local build for this architecture, use it
  local local_file="_dist/irr-${HELM_PLUGIN_VERSION}-${OS}-${ARCH}.tar.gz"
  if [ -f "$local_file" ]; then
    echo "Using local build from $local_file"
    DOWNLOAD_URL="file://$local_file"
    return
  fi

  # Otherwise, use the GitHub API to find the latest version for this project.
  local latest_url="https://api.github.com/repos/$PROJECT_GH/releases/latest"
  if type "curl" > /dev/null; then
    DOWNLOAD_URL=$(curl -s $latest_url | grep $OS | awk '/\"browser_download_url\":/{gsub( /[,\"]/,"", $2); print $2}')
  elif type "wget" > /dev/null; then
    DOWNLOAD_URL=$(wget -q -O - $latest_url | awk '/\"browser_download_url\":/{gsub( /[,\"]/,"", $2); print $2}')
  fi
}

# downloadFile downloads the latest binary package and also the checksum
# for that binary.
downloadFile() {
  PLUGIN_TMP_FILE="/tmp/${PROJECT_NAME}.tgz"
  echo "Downloading $DOWNLOAD_URL"
  if [[ $DOWNLOAD_URL == file://* ]]; then
    cp "${DOWNLOAD_URL#file://}" "$PLUGIN_TMP_FILE"
  elif type "curl" > /dev/null; then
    curl -L "$DOWNLOAD_URL" -o "$PLUGIN_TMP_FILE"
  elif type "wget" > /dev/null; then
    wget -q -O "$PLUGIN_TMP_FILE" "$DOWNLOAD_URL"
  fi
}

# installFile unpacks and installs helm-whatup.
installFile() {
  HELM_TMP="/tmp/$PROJECT_NAME"
  mkdir -p "$HELM_TMP"
  tar xf "$PLUGIN_TMP_FILE" -C "$HELM_TMP"
  echo "Preparing to install into ${HELM_PLUGIN_PATH}"
  cp -R "$HELM_TMP/bin" "$HELM_PLUGIN_PATH/"
}

# fail_trap is executed if an error occurs.
fail_trap() {
  result=$?
  if [ "$result" != "0" ]; then
    echo "Failed to install $PROJECT_NAME"
    echo "\tFor support, go to https://github.com/lucas-albers-lz4/irr."
  fi
  exit $result
}

# testVersion tests the installed client to make sure it is working.
testVersion() {
  set +e
  echo "$PROJECT_NAME installed into $HELM_PLUGIN_PATH/$PROJECT_NAME"
  $HELM_PLUGIN_PATH/bin/$PROJECT_NAME -h
  set -e
}

# Execution

#Stop execution on any error
trap "fail_trap" EXIT
set -e
initArch
initOS
verifySupported
getDownloadURL
downloadFile
installFile
testVersion
