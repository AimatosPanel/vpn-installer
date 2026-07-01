#!/bin/bash
if [ "$EUID" -ne 0 ]; then
  echo "Запустите от имени root."
  exit 1
fi

echo "=== Модуль 3: Память и Хранилище ==="

echo "-> Настройка noatime в /etc/fstab..."
mount -o remount,noatime / 2>/dev/null || true
sed -i '/\s\/\s/ s/defaults/defaults,noatime/g' /etc/fstab 2>/dev/null || true
sed -i '/\s\/\s/ s/relatime/noatime/g' /etc/fstab 2>/dev/null || true

echo "-> Поиск и отключение дискового Swap..."
if [ -f /swapfile ]; then
  swapoff /swapfile || true
  sed -i '/\/swapfile/d' /etc/fstab
  rm -f /swapfile
fi

echo "-> Настройка ZRAM..."
apt update && apt install -y zram-tools

ZRAM_DEFAULT="/etc/default/zram-tools"
cp "$ZRAM_DEFAULT" "${ZRAM_DEFAULT}.bak"

cat <<EOF > "$ZRAM_DEFAULT"
CORES=\$(nproc)
ALGO=zstd
PERCENT=50
PRIORITY=100
EOF

SYSCTL_CONF="/etc/sysctl.conf"
sed -i '/vm.swappiness/d' "$SYSCTL_CONF"
sed -i '/vm.vfs_cache_pressure/d' "$SYSCTL_CONF"
cat <<EOF >> "$SYSCTL_CONF"
vm.swappiness = 15
vm.vfs_cache_pressure = 50
EOF
sysctl -p

systemctl restart zram-tools.service

echo "=== Скрипт 3 выполнен ==="