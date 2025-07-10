#!/bin/bash

PORT=45876
KEY=""
TOKEN=""
HUB_URL=""

usage() {
  printf "Beszel Agent homebrew installation script\n\n"
  printf "Usage: ./install-agent-brew.sh [options]\n\n"
  printf "Options: \n"
  printf "  -k            SSH key (required, or interactive if not provided)\n"
  printf "  -p            Port (default: $PORT)\n"
  printf "  -t            Token (required, or interactive if not provided)\n"
  printf "  -url          Hub URL (required, or interactive if not provided)\n"
  printf "  -h, --help    Display this help message\n"
  exit 0
}

# Handle --help explicitly since getopts doesn't handle long options
if [ "$1" = "--help" ]; then
  usage
fi

# Parse arguments (handling both short and long options)
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

if [ -z "$KEY" ]; then
  read -p "Enter SSH key: " KEY
fi

if [ -z "$TOKEN" ]; then
  read -p "Enter token: " TOKEN
fi

if [ -z "$HUB_URL" ]; then
  read -p "Enter hub URL: " HUB_URL
fi

mkdir -p ~/.config/beszel ~/.cache/beszel

echo "KEY=\"$KEY\"" >~/.config/beszel/beszel-agent.env
echo "LISTEN=$PORT" >>~/.config/beszel/beszel-agent.env
echo "TOKEN=\"$TOKEN\"" >>~/.config/beszel/beszel-agent.env
echo "HUB_URL=\"$HUB_URL\"" >>~/.config/beszel/beszel-agent.env

brew tap henrygd/beszel
brew install beszel-agent
brew services start beszel-agent

printf "\nCheck status: brew services info beszel-agent\n"
echo "Stop: brew services stop beszel-agent"
echo "Start: brew services start beszel-agent"
echo "Restart: brew services restart beszel-agent"
echo "Upgrade: brew upgrade beszel-agent"
echo "Uninstall: brew uninstall beszel-agent"
echo "View logs in ~/.cache/beszel/beszel-agent.log"
printf "Change environment variables in ~/.config/beszel/beszel-agent.env\n"
