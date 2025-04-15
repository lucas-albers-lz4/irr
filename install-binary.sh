#!/bin/bash

# Combination of the Glide and Helm scripts, with my own tweaks.

PROJECT_NAME="irr"
PROJECT_GH="lucas-albers-lz4/$PROJECT_NAME"
HELM_PLUGIN_NAME="irr" # Use the actual plugin name here

# Get plugin path from helm env if HELM_HOME not set
if [ -z "$HELM_HOME" ]; then
  HELM_PLUGINS=$(helm env | grep HELM_PLUGINS | cut -d'"' -f2)
  if [ -n "$HELM_PLUGINS" ]; then
    HELM_PLUGIN_PATH="$HELM_PLUGINS/$HELM_PLUGIN_NAME"
  else
    # Fallback for older Helm versions or if HELM_PLUGINS is not set
    # This might need adjustment based on actual Helm behavior
    DEFAULT_HELM_HOME="$HOME/.helm"
    if helm env | grep -q HELM_PATH_HOME; then
        DEFAULT_HELM_HOME=$(helm env | grep HELM_PATH_HOME | cut -d'"' -f2)
    fi
     HELM_PLUGIN_PATH="$DEFAULT_HELM_HOME/plugins/$HELM_PLUGIN_NAME"
  fi
else
  HELM_PLUGIN_PATH="$HELM_HOME/plugins/$HELM_PLUGIN_NAME"
fi

# Extract version from plugin.yaml if in the same directory
# Use HELM_PLUGIN_DIR which Helm sets for install hooks
if [ -f "$HELM_PLUGIN_DIR/plugin.yaml" ]; then
  HELM_PLUGIN_VERSION=$(grep "version" "$HELM_PLUGIN_DIR/plugin.yaml" | cut -d'"' -f2)
else
  echo "Error: plugin.yaml not found in $HELM_PLUGIN_DIR"
  exit 1
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
  local supported="linux-amd64\nlinux-arm64\ndarwin-arm64" # Updated supported platforms
  if ! echo "${supported}" | grep -q "${OS}-${ARCH}"; then
    echo "No prebuild binary for ${OS}-${ARCH}."
    echo "If you are building from source, try running 'make build' first."
    exit 1
  fi

  if ! type "curl" > /dev/null && ! type "wget" > /dev/null; then
    echo "Either curl or wget is required"
    exit 1
  fi
}

# getDownloadURL checks the latest available version.
getDownloadURL() {
  # Use the GitHub API to find the download URL for this project.
  local latest_url="https://api.github.com/repos/$PROJECT_GH/releases/tags/v${HELM_PLUGIN_VERSION}"
  echo "Fetching release information from $latest_url"

  local download_url_pattern="browser_download_url.*${OS}-${ARCH}\.tar\.gz"

  if type "curl" > /dev/null; then
    DOWNLOAD_URL=$(curl -s $latest_url | grep -o "$download_url_pattern" | cut -d'"' -f4)
  elif type "wget" > /dev/null; then
    DOWNLOAD_URL=$(wget -q -O - $latest_url | grep -o "$download_url_pattern" | cut -d'"' -f4)
  fi

  if [ -z "$DOWNLOAD_URL" ]; then
    echo "Could not find download URL for version v${HELM_PLUGIN_VERSION} and platform ${OS}-${ARCH}"
    echo "Please check https://github.com/$PROJECT_GH/releases"
    exit 1
  fi
}

# downloadFile downloads the binary package and also the checksum
# for that binary.
downloadFile() {
  PLUGIN_TMP_FILE="/tmp/${PROJECT_NAME}.tgz"
  echo "Downloading $DOWNLOAD_URL"
  if type "curl" > /dev/null; then
    curl -L "$DOWNLOAD_URL" -o "$PLUGIN_TMP_FILE"
  elif type "wget" > /dev/null; then
    wget -q -O "$PLUGIN_TMP_FILE" "$DOWNLOAD_URL"
  fi
}

# installFile unpacks and installs the binary.
installFile() {
  HELM_TMP="/tmp/$PROJECT_NAME"
  mkdir -p "$HELM_TMP"
  tar xf "$PLUGIN_TMP_FILE" -C "$HELM_TMP"
  # The binary is expected to be in the 'bin' directory within the tarball
  if [ -f "$HELM_TMP/bin/$PROJECT_NAME" ]; then
    echo "Preparing to install binary into ${HELM_PLUGIN_PATH}/bin"
    mkdir -p "${HELM_PLUGIN_PATH}/bin"
    cp "$HELM_TMP/bin/$PROJECT_NAME" "${HELM_PLUGIN_PATH}/bin/$PROJECT_NAME"
    chmod +x "${HELM_PLUGIN_PATH}/bin/$PROJECT_NAME"
  else
     echo "Error: Binary 'bin/$PROJECT_NAME' not found in downloaded archive."
     exit 1
  fi
  # Clean up tmp files
  rm -f "$PLUGIN_TMP_FILE"
  rm -rf "$HELM_TMP"
}

# fail_trap is executed if an error occurs.
fail_trap() {
  result=$?
  if [ "$result" != "0" ]; then
    echo "Failed to install $PROJECT_NAME plugin."
    echo -e "\tFor support, go to https://github.com/$PROJECT_GH."
  fi
  # Clean up potentially leftover tmp files
  rm -f "/tmp/${PROJECT_NAME}.tgz"
  rm -rf "/tmp/${PROJECT_NAME}"
  exit $result
}

# testVersion tests the installed client to make sure it is working.
testVersion() {
  set +e # Allow command to fail without exiting script
  echo "$PROJECT_NAME installed into $HELM_PLUGIN_PATH"
  echo "Running '$PROJECT_NAME version' to verify installation..."
  "${HELM_PLUGIN_PATH}/bin/$PROJECT_NAME" version
  if [ "$?" != "0" ]; then
      echo "Could not run '$PROJECT_NAME version'. Installation may have failed."
      exit 1
  fi
  echo "'$PROJECT_NAME version' command executed successfully."
  set -e
}

# Execution

#Stop execution on any error
trap "fail_trap" EXIT
set -e

echo "Installing $PROJECT_NAME Helm plugin..."
initArch
initOS
verifySupported
getDownloadURL # This now gets the URL based on the version in plugin.yaml
downloadFile
installFile
testVersion

echo "$PROJECT_NAME plugin installed successfully."
echo "You can now run commands like: helm $PROJECT_NAME --help" 