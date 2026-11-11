#!/bin/bash
# Launcher for beszel-agent, installed alongside the agent binary by
# Homebrew formulas (`henrygd/beszel` tap and homebrew-core) so that
# `brew services` runs the agent with the user's configuration.
#
# Sources environment variables (KEY, TOKEN, HUB_URL, ...) from the env
# file created by install-agent-brew.sh, then executes the beszel-agent
# binary located in the same directory as this script.

# Export all variables defined in the env file.
set -a
if [ -f "$HOME/.config/beszel/beszel-agent.env" ]; then
  source "$HOME/.config/beszel/beszel-agent.env"
fi
set +a

exec "$(dirname "$0")/beszel-agent" "$@"
