#!/bin/bash

PORT=45876
KEY=""

usage() {
  printf "Beszel Agent homebrew installation script\n\n"
  printf "Usage: ./install-agent-brew.sh [options]\n\n"
  printf "Options: \n"
  printf "  -k            SSH key (required, or interactive if not provided)\n"
  printf "  -p            Port (default: $PORT)\n"
  printf "  -h, --help    Display this help message\n"
  exit 0
}

# Handle --help explicitly since getopts doesn't handle long options
if [ "$1" = "--help" ]; then
  usage
fi

# Parse arguments with getopts
while getopts "k:p:h" opt; do
  case ${opt} in
  k)
    KEY="$OPTARG"
    ;;
  p)
    PORT="$OPTARG"
    ;;
  h)
    usage
    ;;
  \?)
    echo "Invalid option: -$OPTARG" >&2
    usage
    ;;
  :)
    echo "Option -$OPTARG requires an argument." >&2
    usage
    ;;
  esac
done

# Exit if brew is not installed
if ! command -v brew &>/dev/null; then
  echo "Homebrew is not installed. Please install Homebrew and try again."
  exit 1
fi

if [ -z "$KEY" ]; then
  read -p "Enter SSH key: " KEY
fi

mkdir -p ~/.config/beszel ~/.cache/beszel

echo "KEY=\"$KEY\"" >~/.config/beszel/beszel-agent.env
echo "PORT=$PORT" >>~/.config/beszel/beszel-agent.env

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
echo "Change environment variables in ~/.config/beszel/beszel-agent.env"
