#!/bin/bash
set -eo pipefail

CLR_PURPLE="\033[1;35m"
CLR_GREEN="\033[1;32m"
CLR_RESET="\033[0m"

echo -e "\n${CLR_PURPLE}════ [ Модуль 3: Память ZRAM и быстрый доступ NVMe/SSD ] ════${CLR_RESET}"

# 1. noatime
echo -e " ${CLR_PURPLE}➔${CLR_RESET} Активация режима noatime для снижения износа диска..."
mount -o remount,noatime / 2>/dev/null || true
sed -i '/\s\/\s/ s/defaults/defaults,noatime/g' /etc/fstab 2>/dev/null || true
sed -i '/\s\/\s/ s/relatime/noatime/g' /etc/fstab 2>/dev/null || true

# 2. Отключение медленного дискового Swap
echo -e " ${CLR_PURPLE}➔${CLR_RESET} Проверка и отключение дискового swap..."
if [[ -f /swapfile ]]; then
    swapoff /swapfile 2>/dev/null || true
    sed -i '\|^/swapfile|d' /etc/fstab 2>/dev/null || true
    rm -f /swapfile
fi

# 3. ZRAM сжатие в RAM (ZSTD)
echo -e " ${CLR_PURPLE}➔${CLR_RESET} Развёртывание пула сжатия ZRAM (алгоритм zstd, 50% RAM)..."
apt-get update -y >/dev/null 2>&1 && apt-get install -y zram-tools >/dev/null 2>&1

cat <<'EOF' > /etc/default/zram-tools
CORES=$(nproc)
ALGO=zstd
PERCENT=50
PRIORITY=100
EOF

SYSCTL_CONF="/etc/sysctl.conf"
sed -i '/vm.swappiness/d' "$SYSCTL_CONF"
sed -i '/vm.vfs_cache_pressure/d' "$SYSCTL_CONF"
echo "vm.swappiness = 15" >> "$SYSCTL_CONF"
echo "vm.vfs_cache_pressure = 50" >> "$SYSCTL_CONF"
sysctl -p >/dev/null 2>&1

systemctl restart zram-tools.service 2>/dev/null || true
echo -e " ${CLR_GREEN}✓ Модуль 3 успешно применён!${CLR_RESET}"