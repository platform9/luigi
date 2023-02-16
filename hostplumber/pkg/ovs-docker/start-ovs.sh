#!/bin/bash

CONFIG_FILE=/etc/openvswitch/ovs.conf
export PATH=$PATH:/usr/local/share/openvswitch/scripts
ovs-ctl start

# Start ovsdb
ovs-ctl restart --no-ovs-vswitchd --system-id=random

if $EnableDpdk == true ; then
	printf "Setting DPDK configuration options..."
	# Read the config and setup OVS
	while IFS= read -r config_line
	do
		if [[ $config_line ]] && [[ $config_line != \#* ]] ; then
			ovs-vsctl --no-wait set Open_vSwitch . other_config:$config_line
		fi	
	done < "$CONFIG_FILE"
fi

# Start vswitchd
ovs-ctl restart --no-ovsdb-server --system-id=random

if [[ $(ovs-vsctl get Open_vSwitch . dpdk_initialized) == true ]] ; then echo "DPDK EAL initialization succeeded..."; fi
