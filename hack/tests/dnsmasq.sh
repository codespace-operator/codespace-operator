# In WSL (Ubuntu/Debian)
sudo apt-get update && sudo apt-get install -y dnsmasq

# Basic config
sudo tee /etc/dnsmasq.d/kind.conf >/dev/null <<'CONF'
# listen only on loopback
listen-address=127.0.0.1
bind-interfaces
# forward everything else to a real resolver
server=1.1.1.1
# wildcard: anything.dev.test -> 127.0.0.1
address=/.dev.test/127.0.0.1
CONF

sudo systemctl enable --now dnsmasq

# /etc/wsl.conf
sudo tee /etc/wsl.conf >/dev/null <<'CONF'
[network]
generateResolvConf = false
CONF

# Replace resolv.conf and restart WSL
sudo rm -f /etc/resolv.conf
echo "nameserver 127.0.0.1" | sudo tee /etc/resolv.conf

nslookup demo.dev.test 127.0.0.1
curl -H "Host: demo.dev.test" http://127.0.0.1/
