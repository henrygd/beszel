#!/bin/sh
set -e

[ "$1" = "configure" ] || exit 0

CONFIG_FILE=/etc/beszel-agent.conf
SERVICE=beszel-agent
SERVICE_USER=beszel

. /usr/share/debconf/confmodule

# Create group and user
if ! getent group "$SERVICE_USER" >/dev/null; then
	echo "Creating $SERVICE_USER group"
	addgroup --quiet --system "$SERVICE_USER"
fi

if ! getent passwd "$SERVICE_USER" >/dev/null; then
	echo "Creating $SERVICE_USER user"
	adduser --quiet --system "$SERVICE_USER" \
		--ingroup "$SERVICE_USER" \
		--no-create-home \
		--home /nonexistent \
		--gecos "System user for $SERVICE"
fi

# Enable docker (only if docker group exists)
if getent group docker >/dev/null 2>&1; then
	if ! getent group docker | grep -q "$SERVICE_USER"; then
		echo "Adding $SERVICE_USER to docker group"
		usermod -aG docker "$SERVICE_USER"
	fi
fi

# Create config file if it doesn't already exist
if [ ! -f "$CONFIG_FILE" ]; then
	touch "$CONFIG_FILE"
	chmod 0600 "$CONFIG_FILE"
	chown "$SERVICE_USER":"$SERVICE_USER" "$CONFIG_FILE"
fi;

# Only add key to config if it's not already present
if ! grep -q "^KEY=" "$CONFIG_FILE"; then
	db_get beszel-agent/key
	echo "KEY=$RET" > "$CONFIG_FILE"
fi;

deb-systemd-helper enable "$SERVICE".service
systemctl daemon-reload
deb-systemd-invoke start "$SERVICE".service || echo "could not start $SERVICE.service!"
