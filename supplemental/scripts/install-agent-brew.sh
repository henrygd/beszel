#!/bin/bash

PORT=45876
KEY=""

usage() {
  printf "CMonitor Agent homebrew installation script\n\n"
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

mkdir -p ~/.config/cmonitor ~/.cache/cmonitor

echo "KEY=\"$KEY\"" >~/.config/cmonitor/cmonitor-agent.env
echo "LISTEN=$PORT" >>~/.config/cmonitor/cmonitor-agent.env

brew tap henrygd/cmonitor
brew install cmonitor-agent
brew services start cmonitor-agent

printf "\nCheck status: brew services info cmonitor-agent\n"
echo "Stop: brew services stop cmonitor-agent"
echo "Start: brew services start cmonitor-agent"
echo "Restart: brew services restart cmonitor-agent"
echo "Upgrade: brew upgrade cmonitor-agent"
echo "Uninstall: brew uninstall cmonitor-agent"
echo "View logs in ~/.cache/cmonitor/cmonitor-agent.log"
printf "Change environment variables in ~/.config/cmonitor/cmonitor-agent.env\n"