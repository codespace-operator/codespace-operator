#!/bin/bash
# Enhanced dnsmasq setup for WSL + kind

# In WSL (Ubuntu/Debian)
sudo apt-get update && sudo apt-get install -y dnsmasq

# More comprehensive dnsmasq config
sudo tee /etc/dnsmasq.d/kind.conf >/dev/null <<'CONF'
# Listen on loopback and WSL interface
listen-address=127.0.0.1
bind-interfaces

# Forward to multiple DNS servers for reliability
server=1.1.1.1
server=8.8.8.8

# Wildcard domains for development
address=/.codespace.test/127.0.0.1
address=/.dev.test/127.0.0.1
address=/.localhost/127.0.0.1

# Cache settings
cache-size=1000
CONF

# Ensure dnsmasq starts properly
sudo systemctl stop systemd-resolved || true
sudo systemctl disable systemd-resolved || true
sudo systemctl enable dnsmasq
sudo systemctl restart dnsmasq

# Update WSL config
sudo tee /etc/wsl.conf >/dev/null <<'CONF'
[network]
generateResolvConf = false
hostname = wsl-codespace

[interop]
enabled = true
appendWindowsPath = true
CONF

# Set up proper DNS resolution
sudo rm -f /etc/resolv.conf
sudo tee /etc/resolv.conf >/dev/null <<'CONF'
nameserver 127.0.0.1
nameserver 1.1.1.1
nameserver 8.8.8.8
CONF

# Make resolv.conf immutable to prevent overwriting
sudo chattr +i /etc/resolv.conf 2>/dev/null || true

echo ">>> Testing DNS resolution..."
nslookup console.codespace.test 127.0.0.1