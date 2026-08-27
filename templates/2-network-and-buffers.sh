#!/bin/bash
set -eo pipefail

CLR_PURPLE="\033[1;35m"
CLR_GREEN="\033[1;32m"
CLR_RESET="\033[0m"

echo -e "\n${CLR_PURPLE}════ [ Модуль 2: Тюнинг сетевых буферов и TCP BBR ] ════${CLR_RESET}"

apt-get update -y >/dev/null 2>&1 && apt-get install -y ethtool >/dev/null 2>&1

# 1. Оптимизация сетевых очередей Ring Buffer
echo -e " ${CLR_PURPLE}➔${CLR_RESET} Тюнинг Ring Buffer сетевых адаптеров..."
for iface in $(ls /sys/class/net); do
    if [[ "$iface" != "lo" && "$iface" != wg* && "$iface" != tun* ]]; then
        ethtool -G "$iface" rx 2048 tx 2048 2>/dev/null || ethtool -G "$iface" rx 1024 tx 1024 2>/dev/null || true
    fi
done

cat <<'EOF' > /etc/udev/rules.d/98-ring-buffers.rules
ACTION=="add|change", SUBSYSTEM=="net", KERNEL=="eth*|ens*|enp*|en*", RUN+="/usr/sbin/ethtool -G %k rx 2048 tx 2048"
EOF

# 2. Идемпотентная настройка sysctl
echo -e " ${CLR_PURPLE}➔${CLR_RESET} Применение оптимизаций ядра sysctl (BBR + расширенные буферы)..."
SYSCTL_CONF="/etc/sysctl.conf"
[[ ! -f "${SYSCTL_CONF}.bak" ]] && cp "$SYSCTL_CONF" "${SYSCTL_CONF}.bak"

# Удаляем старые записи Aimatos во избежание дубликатов
sed -i '/# AIMATOS ADVANCED NETWORK START/,/# AIMATOS ADVANCED NETWORK END/d' "$SYSCTL_CONF"

cat <<'EOF' >> "$SYSCTL_CONF"
# AIMATOS ADVANCED NETWORK START
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
# AIMATOS ADVANCED NETWORK END
EOF

sysctl -p >/dev/null 2>&1
echo -e " ${CLR_GREEN}✓ Модуль 2 успешно применён!${CLR_RESET}"