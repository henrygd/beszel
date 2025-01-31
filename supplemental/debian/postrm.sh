#!/bin/sh
set -e

if [ "$1" = "purge" ]; then
	. /usr/share/debconf/confmodule
	db_purge
	rm /etc/beszel-agent.conf
fi
