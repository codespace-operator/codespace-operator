mkdir -p local
# if your kind supports it (most do):
kind get kubeconfig --name codespace --internal > ./local/kubeconfig
# if --internal isn’t supported, fallback:
kind get kubeconfig --name codespace > ./local/kubeconfig
# replace the server line to reach the control plane by DNS on the kind network:
sed -i 's#https://127.0.0.1:[0-9]*#https://codespace-control-plane:6443#g' ./local/kubeconfig
chmod 644 ./local/kubeconfig

docker compose up