#!/bin/bash
set -euo pipefail

# Mode: default to test unless "commit" is passed
MODE=${1:-"test"}

echo "Running in $MODE mode..."

# 1. Reset UFW to default state
echo "Resetting UFW rules..."
ufw --force reset

# 2. Set default policies
echo "Setting default policies..."
ufw default deny incoming
ufw default allow outgoing
ufw default allow forward

# 3. Allow loopback
echo "Allowing loopback..."
ufw allow in on lo
ufw allow out on lo

# 4. Allow CNI and Flannel interfaces
echo "Allowing CNI (cni0) and Flannel (flannel.1) interfaces..."
ufw allow in on cni0
ufw allow out on cni0
ufw allow in on flannel.1
ufw allow out on flannel.1

# Allow Kubernetes subnets (pods and services)
ufw allow in from 10.42.0.0/16
ufw allow out to 10.42.0.0/16
ufw allow in from 10.43.0.0/16
ufw allow out to 10.43.0.0/16

# 5. Allow SSH (Port 22) from the internal private network
echo "Allowing SSH from 192.168.100.0/24..."
ufw allow proto tcp from 192.168.100.0/24 to any port 22 comment 'Allow SSH from internal network'

# 6. Allow Ingress Traffic (HTTP/HTTPS) from any source
echo "Allowing Ingress (80/443)..."
ufw allow proto tcp from any to any port 80 comment 'Allow Traefik HTTP Ingress'
ufw allow proto tcp from any to any port 443 comment 'Allow Traefik HTTPS Ingress'

# 7. Allow K3s node-specific communication from Master (192.168.100.241)
echo "Allowing K3s traffic from master (192.168.100.241)..."
ufw allow proto tcp from 192.168.100.241 to any port 10250 comment 'Allow kubelet API access'
ufw allow proto udp from 192.168.100.241 to any port 8472 comment 'Allow Flannel VXLAN'

# 8. Enable UFW
if [ "$MODE" = "commit" ]; then
    echo "Applying rules permanently..."
    ufw --force enable
    echo "UFW rules applied permanently. Status:"
    ufw status verbose
else
    echo "Applying rules in TEST mode..."
    ufw --force enable
    echo "UFW enabled. Waiting 60 seconds before automatic rollback..."
    for i in {60..1}; do
        echo -ne "Rollback in $i seconds... \r"
        sleep 1
    done
    echo ""
    echo "Disabling UFW..."
    ufw disable
    echo "UFW disabled. Rollback complete."
fi
