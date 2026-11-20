#!/bin/bash

# Installs the beszel-agent formula from homebrew-core and runs it with
# `brew services`, reading configuration from Homebrew's per-service env file:
# $HOMEBREW_USER_CONFIG_HOME/services/beszel-agent.env (default ~/.homebrew).
#
# Also migrates installs made by earlier versions of this script, which used
# the henrygd/beszel tap and ~/.config/beszel/beszel-agent.env.

PORT=""
KEY=""
TOKEN=""
HUB_URL=""

DEFAULT_PORT=45876
# First Homebrew release that reads services/<formula>.env
MIN_BREW_VERSION="6.0.15"
OLD_TAP="henrygd/beszel"
OLD_CONFIG_DIR="$HOME/.config/beszel"
OLD_ENV_FILE="$OLD_CONFIG_DIR/beszel-agent.env"

usage() {
  printf "Beszel Agent homebrew installation script\n\n"
  printf "Usage: ./install-agent-brew.sh [options]\n\n"
  printf "Options: \n"
  printf "  -k            SSH key (required, or interactive if not provided)\n"
  printf "  -p            Port (default: $DEFAULT_PORT)\n"
  printf "  -t            Token (optional for backwards compatibility)\n"
  printf "  -url          Hub URL (optional for backwards compatibility)\n"
  printf "  -h, --help    Display this help message\n\n"
  printf "Options not provided are read from an existing configuration if present.\n"
  printf "Run without options to migrate an existing install from the $OLD_TAP tap.\n"
  exit 0
}

# Parse arguments
while [ $# -gt 0 ]; do
  case "$1" in
  -k)
    shift
    KEY="$1"
    ;;
  -p)
    shift
    PORT="$1"
    ;;
  -t)
    shift
    TOKEN="$1"
    ;;
  -url)
    shift
    HUB_URL="$1"
    ;;
  -h | --help)
    usage
    ;;
  *)
    echo "Invalid option: $1" >&2
    usage
    ;;
  esac
  shift
done

# Prints the KEY=value lines of an env file in the format brew services expects:
# no quotes around values, no `export`, no comments or blank lines.
# Pass "warn" as the second argument to flag values that relied on the old
# launcher sourcing the file as a shell script.
normalize_env_file() {
  local line key value
  while IFS= read -r line || [ -n "$line" ]; do
    # trim leading and trailing whitespace
    line="${line#"${line%%[![:space:]]*}"}"
    line="${line%"${line##*[![:space:]]}"}"
    case "$line" in "" | \#*) continue ;; esac
    line="${line#export }"
    case "$line" in
    *=*) ;;
    *)
      echo "Warning: skipping invalid line in $1: $line" >&2
      continue
      ;;
    esac
    key="${line%%=*}"
    key="${key%"${key##*[![:space:]]}"}"
    value="${line#*=}"
    value="${value#"${value%%[![:space:]]*}"}"
    # brew services uses values as-is, so remove surrounding quotes
    case "$value" in
    \"*\" | \'*\') value="${value:1:$((${#value} - 2))}" ;;
    esac
    # brew strips whitespace around values, so match it
    value="${value#"${value%%[![:space:]]*}"}"
    value="${value%"${value##*[![:space:]]}"}"
    [ "$2" = warn ] && case "$value" in
    *'$'* | *'`'*) echo "Warning: $key in $1 contains shell syntax that will not be expanded" >&2 ;;
    esac
    printf '%s=%s\n' "$key" "$value"
  done <"$1"
}

# Prints the value of a key from normalized env content.
env_get() {
  printf '%s\n' "$1" | sed -n "s/^$2=//p" | tail -n 1
}

# Returns 0 if brew is at least the given version, 1 if older, 2 if unknown.
brew_version_at_least() {
  local version i
  version=$(brew --version 2>/dev/null | head -n 1 | sed -nE 's/^Homebrew ([0-9]+\.[0-9]+\.[0-9]+).*/\1/p')
  [ -z "$version" ] && return 2
  local IFS=.
  local -a have=($version) want=($1)
  for i in 0 1 2; do
    if [ "${have[$i]}" -gt "${want[$i]}" ]; then return 0; fi
    if [ "${have[$i]}" -lt "${want[$i]}" ]; then return 1; fi
  done
  return 0
}

# Returns 0 if the installed beszel-agent came from the henrygd/beszel tap.
is_tap_install() {
  local opt="$BREW_PREFIX/opt/beszel-agent"
  [ -e "$opt/bin/beszel-agent-launcher" ] && return 0
  grep -qE "\"tap\": *\"$OLD_TAP\"" "$opt/INSTALL_RECEIPT.json" 2>/dev/null
}

# Check if brew is installed, prompt to install if not
if ! command -v brew &>/dev/null; then
  read -p "Homebrew is not installed. Would you like to install it now? (y/n): " install_brew
  if [[ $install_brew =~ ^[Yy]$ ]]; then
    echo "Installing Homebrew..."
    /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"

    # Verify installation was successful
    if ! command -v brew &>/dev/null; then
      echo "Homebrew installation failed. Please install manually and try again."
      exit 1
    fi
    echo "Homebrew installed successfully."
  else
    echo "Homebrew is required. Please install Homebrew and try again."
    exit 1
  fi
fi

# Older Homebrew versions silently ignore the services env file
brew_version_at_least "$MIN_BREW_VERSION"
case $? in
1)
  echo "Updating Homebrew (version $MIN_BREW_VERSION or newer is required)..."
  brew update
  if ! brew_version_at_least "$MIN_BREW_VERSION"; then
    echo "Homebrew $MIN_BREW_VERSION or newer is required. Please update Homebrew and try again." >&2
    exit 1
  fi
  ;;
2)
  echo "Warning: could not determine Homebrew version. Version $MIN_BREW_VERSION or newer is required." >&2
  ;;
esac

BREW_PREFIX="$(brew --prefix)"

# Same logic Homebrew uses to locate the user config directory
if [ -n "${XDG_CONFIG_HOME-}" ]; then
  BREW_USER_CONFIG_HOME="$XDG_CONFIG_HOME/homebrew"
elif [ -n "${HOMEBREW_XDG_CONFIG_HOME-}" ]; then
  BREW_USER_CONFIG_HOME="$HOMEBREW_XDG_CONFIG_HOME/homebrew"
else
  BREW_USER_CONFIG_HOME="$HOME/.homebrew"
fi
ENV_FILE="$BREW_USER_CONFIG_HOME/services/beszel-agent.env"
DATA_DIR="$BREW_PREFIX/var/lib/beszel-agent"

# Load existing configuration, preferring the current location over the old one
EXISTING_ENV=""
MIGRATED_ENV=false
if [ -f "$ENV_FILE" ]; then
  EXISTING_ENV="$(normalize_env_file "$ENV_FILE")"
elif [ -f "$OLD_ENV_FILE" ]; then
  echo "Migrating configuration from $OLD_ENV_FILE"
  EXISTING_ENV="$(normalize_env_file "$OLD_ENV_FILE" warn)"
  MIGRATED_ENV=true
fi

# Arguments take precedence over existing values
[ -z "$KEY" ] && KEY="$(env_get "$EXISTING_ENV" KEY)"
[ -z "$PORT" ] && PORT="$(env_get "$EXISTING_ENV" LISTEN)"
[ -z "$PORT" ] && PORT="$DEFAULT_PORT"
[ -z "$TOKEN" ] && TOKEN="$(env_get "$EXISTING_ENV" TOKEN)"
[ -z "$HUB_URL" ] && HUB_URL="$(env_get "$EXISTING_ENV" HUB_URL)"

if [ -z "$KEY" ]; then
  read -p "Enter SSH key: " KEY
fi
if [ -z "$KEY" ]; then
  echo "An SSH key is required." >&2
  exit 1
fi

# TOKEN and HUB_URL are optional for backwards compatibility - no interactive prompts

# Values are written without quotes because brew services uses them as-is
NEW_ENV="KEY=$KEY"$'\n'"LISTEN=$PORT"
[ -n "$TOKEN" ] && NEW_ENV+=$'\n'"TOKEN=$TOKEN"
[ -n "$HUB_URL" ] && NEW_ENV+=$'\n'"HUB_URL=$HUB_URL"
# Keep any other variables from the existing configuration
EXTRA_ENV="$(printf '%s\n' "$EXISTING_ENV" | grep -vE '^(KEY|LISTEN|TOKEN|HUB_URL)=' | grep -v '^$')"
[ -n "$EXTRA_ENV" ] && NEW_ENV+=$'\n'"$EXTRA_ENV"

mkdir -p "$(dirname "$ENV_FILE")"
# brew services ignores the file if it is group or world writable
(umask 077 && printf '%s\n' "$NEW_ENV" >"$ENV_FILE")
chmod 600 "$ENV_FILE"

# Replace a previous install from the henrygd/beszel tap with homebrew-core
if is_tap_install; then
  echo "Replacing beszel-agent from the $OLD_TAP tap with the homebrew-core formula..."
  brew services stop beszel-agent >/dev/null 2>&1
  if ! brew uninstall beszel-agent; then
    echo "Failed to uninstall the previous beszel-agent. Please run 'brew uninstall beszel-agent' and try again." >&2
    exit 1
  fi
fi
if brew tap | grep -qx "$OLD_TAP"; then
  brew untap "$OLD_TAP" >/dev/null 2>&1
fi

if ! brew install beszel-agent; then
  echo "Failed to install beszel-agent. Please run 'brew install beszel-agent', then run this script again." >&2
  exit 1
fi

# Keep the system's existing fingerprint from the old data directory
if [ -f "$OLD_CONFIG_DIR/fingerprint" ] && [ ! -f "$DATA_DIR/fingerprint" ]; then
  mkdir -p "$DATA_DIR" && cp "$OLD_CONFIG_DIR/fingerprint" "$DATA_DIR/fingerprint"
fi

# restart regenerates the service definition so it picks up the env file
brew services restart beszel-agent

printf "\nCheck status: brew services info beszel-agent\n"
echo "Stop: brew services stop beszel-agent"
echo "Start: brew services start beszel-agent"
echo "Restart: brew services restart beszel-agent"
echo "Upgrade: brew upgrade beszel-agent"
echo "Uninstall: brew uninstall beszel-agent"
echo "View logs in $BREW_PREFIX/var/log/beszel-agent.log"
echo "Change environment variables in $ENV_FILE"
printf "Do not use quotes around values, and restart the service after making changes.\n"
if [ "$MIGRATED_ENV" = true ]; then
  printf "\n%s is no longer used and can be removed.\n" "$OLD_ENV_FILE"
fi
