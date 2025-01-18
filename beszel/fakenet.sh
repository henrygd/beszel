#!/bin/bash

OUTPUT="/dev/shm/fakenet/";

if [ ! -d "$OUTPUT" ]; then
	mkdir -p $OUTPUT/{rpc,stat,ip_vs}
fi

SOURCES="/proc/net/dev
/proc/net/ip_vs_stats
/proc/net/ip_vs/stats
/proc/net/netstat
/proc/net/rpc/nfs
/proc/net/rpc/nfsd
/proc/net/snmp
/proc/net/snmp6
/proc/net/softnet_stat
/proc/net/stat/nf_conntrack
/proc/net/stat/conntrack
/proc/net/stat/synproxy"

NETFILES=""
for SOURCE in $SOURCES; do
	if [ -f "$SOURCE" ]; then
		NETFILES="$NETFILES $SOURCE"
	fi
done

while [ true ]; do
echo "Run ..."
	for NETFILE in $NETFILES; do
		OUTFILE="${NETFILE:10}"
		echo "$(<$NETFILE)" > $OUTPUT$OUTFILE
	done
	##date +%s.%N
	sleep 0.23
done
