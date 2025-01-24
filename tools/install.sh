#!/bin/sh
#
# This script should be run via curl:
#   sh -c "$(curl -fsSL https://raw.githubusercontent.com/loopwork-ai/emcee/main/tools/install.sh)"
# or via wget:
#   sh -c "$(wget -qO- https://raw.githubusercontent.com/loopwork-ai/emcee/main/tools/install.sh)"
# or via fetch:
#   sh -c "$(fetch -o - https://raw.githubusercontent.com/loopwork-ai/emcee/main/tools/install.sh)"
#
# As an alternative, you can first download the install script and run it afterwards:
#   wget https://raw.githubusercontent.com/loopwork-ai/emcee/main/tools/install.sh
#   sh install.sh
#
# You can tweak the install location by setting the INSTALL_DIR env var when running the script.
#   INSTALL_DIR=~/my/custom/install/location sh install.sh
#
# By default, emcee will be installed at /usr/local/bin/emcee
#
# This install script is based on that of ohmyzsh[1], which is licensed under the MIT License
# [1] https://github.com/ohmyzsh/ohmyzsh/blob/master/tools/install.sh

set -e

# Make sure important variables exist if not already defined
DEFAULT_INSTALL_DIR="/usr/local/bin"
INSTALL_DIR=${INSTALL_DIR:-$DEFAULT_INSTALL_DIR}

command_exists() {
  command -v "$@" >/dev/null 2>&1
}

user_can_sudo() {
  # Check if sudo is installed
  command_exists sudo || return 1
  # Termux can't run sudo, so we can detect it and exit the function early.
  case "$PREFIX" in
    *com.termux*) return 1 ;;
  esac
  ! LANG= sudo -n -v 2>&1 | grep -q "may not run sudo"
}

setup_color() {
  # Only use colors if connected to a terminal
  if [ -t 1 ]; then
    FMT_RED=$(printf '\033[31m')
    FMT_GREEN=$(printf '\033[32m')
    FMT_YELLOW=$(printf '\033[33m')
    FMT_BLUE=$(printf '\033[34m')
    FMT_BOLD=$(printf '\033[1m')
    FMT_RESET=$(printf '\033[0m')
  else
    FMT_RED=""
    FMT_GREEN=""
    FMT_YELLOW=""
    FMT_BLUE=""
    FMT_BOLD=""
    FMT_RESET=""
  fi
}

get_platform() {
  platform="$(uname -s)_$(uname -m)"
  case "$platform" in
    "Darwin_arm64"|"Darwin_x86_64"|"Linux_arm64"|"Linux_i386"|"Linux_x86_64")
      echo "$platform"
      return 0
      ;;
    *)
      echo "Unsupported platform: $platform"
      return 1
      ;;
  esac
}

setup_emcee() {
  EMCEE_LOCATION="${INSTALL_DIR}/emcee"
  platform=$(get_platform) || exit 1
  
  BINARY_URI="https://github.com/loopwork-ai/emcee/releases/latest/download/emcee_${platform}.tar.gz"

  if [ -f "$EMCEE_LOCATION" ]; then
    echo "${FMT_YELLOW}A file already exists at $EMCEE_LOCATION${FMT_RESET}"
    printf "${FMT_YELLOW}Do you want to delete this file and continue with this installation anyway?${FMT_RESET}\n"
    read -p "Delete file? (y/N): " choice
    case "$choice" in
      y|Y ) echo "Deleting existing file and continuing with installation..."; sudo rm $EMCEE_LOCATION;;
      * ) echo "Exiting installation."; exit 1;;
    esac
  fi

  echo "${FMT_BLUE}Downloading emcee...${FMT_RESET}"
  
  TMP_DIR=$(mktemp -d)
  if command_exists curl; then
    curl -L "$BINARY_URI" | tar xz -C "$TMP_DIR"
  elif command_exists wget; then
    wget -O- "$BINARY_URI" | tar xz -C "$TMP_DIR"
  elif command_exists fetch; then
    fetch -o- "$BINARY_URI" | tar xz -C "$TMP_DIR"
  else
    echo "${FMT_RED}Error: One of curl, wget, or fetch must be present for this installer to work.${FMT_RESET}"
    exit 1
  fi

  sudo mv "$TMP_DIR/emcee" "$EMCEE_LOCATION"
  rm -rf "$TMP_DIR"
  sudo chmod +x "$EMCEE_LOCATION"

  SHELL_NAME=$(basename "$SHELL")
  if ! echo "$PATH" | grep -q "$INSTALL_DIR"; then
    echo "Adding $INSTALL_DIR to PATH in .$SHELL_NAME"rc
    echo "" >> ~/.$SHELL_NAME"rc"
    echo "# Created by \`emcee\` install script on $(date)" >> ~/.$SHELL_NAME"rc"
    echo "export PATH=\$PATH:$INSTALL_DIR" >> ~/.$SHELL_NAME"rc"
    echo "You may need to open a new terminal window to run emcee for the first time."
  fi

  trap 'rm -rf "$TMP_DIR"' EXIT
}

print_success() {
  echo "${FMT_GREEN}Successfully installed emcee.${FMT_RESET}"
  echo "${FMT_BLUE}Run \`emcee --help\` to get started${FMT_RESET}"
}

main() {
  setup_color

  # Check if `emcee` command already exists
  if command_exists emcee; then
    echo "${FMT_YELLOW}An emcee command already exists on your system at the following location: $(which emcee)${FMT_RESET}"
    echo "The installations may interfere with one another."
    printf "${FMT_YELLOW}Do you want to continue with this installation anyway?${FMT_RESET}\n"
    read -p "Continue? (y/N): " choice
    case "$choice" in
      y|Y ) echo "Continuing with installation...";;
      * ) echo "Exiting installation."; exit 1;;
    esac
  fi

  # Check the users sudo privileges
  if [ ! "$(user_can_sudo)" ] && [ "${SUDO}" != "" ]; then
    echo "${FMT_RED}You need sudo permissions to run this install script. Please try again as a sudoer.${FMT_RESET}"
    exit 1
  fi

  setup_emcee

  if command_exists emcee; then
    print_success
  else
    echo "${FMT_RED}Error: emcee not installed.${FMT_RESET}"
    exit 1
  fi
}

main "$@"