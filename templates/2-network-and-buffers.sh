#!/bin/bash
if [ "$EUID" -ne 0 ]; then
  echo "Запустите от имени root."
  exit 1
fi

echo "=== Модуль 2: Тюнинг Сети ==="

apt update && apt install -y ethtool

echo "-> Оптимизация Ring Buffer..."
for iface in $(ls /sys/class/net); do
    if [[ "$iface" != "lo" && "$iface" != wg* && "$iface" != tun* ]]; then
        ethtool -G "$iface" rx 2048 tx 2048 2>/dev/null || ethtool -G "$iface" rx 1024 tx 1024 2>/dev/null || true
    fi
done

cat <<EOF > /etc/udev/rules.d/98-ring-buffers.rules
ACTION=="add|change", SUBSYSTEM=="net", KERNEL=="eth*|ens*|enp*|en*", RUN+="/usr/sbin/ethtool -G %k rx 2048 tx 2048"
EOF

echo "-> Тюнинг sysctl..."
SYSCTL_CONF="/etc/sysctl.conf"
cp "$SYSCTL_CONF" "${SYSCTL_CONF}.bak"
sed -i '/# VPN Advanced/,$d' "$SYSCTL_CONF"

cat <<EOF >> "$SYSCTL_CONF"

# VPN Advanced Start
net.ipv4.ip_forward = 1
net.ipv6.conf.all.forwarding = 1
net.core.default_qdisc = fq
net.ipv4.tcp_congestion_control = bbr
net.core.netdev_max_backlog = 100000
net.unix.max_dgram_qlen = 512
net.core.rmem_max = 33554432
net.core.wmem_max = 33554432
net.core.rmem_default = 16777216
net.core.wmem_default = 16777216
net.ipv4.tcp_rmem = 4096 87380 33554432
net.ipv4.tcp_wmem = 4096 65536 33554432
net.ipv4.udp_rmem_min = 16384
net.ipv4.udp_wmem_min = 16384
net.netfilter.nf_conntrack_max = 1048576
net.netfilter.nf_conntrack_tcp_timeout_established = 600
net.ipv4.tcp_slow_start_after_idle = 0
# VPN Advanced End
EOF

sysctl -p

echo "=== Скрипт 2 выполнен ==="