#!/bin/bash
set -e

if [ "$EUID" -ne 0 ]; then
  echo "❌ Ошибка: Скрипт должен быть запущен с правами root."
  exit 1
fi

echo "=================================================="
echo "🛸 Запуск мастера полного удаления AimatosPanel..."
echo "=================================================="

# 1. Остановка процессов и служб
echo "-> Остановка фоновых служб..."
systemctl stop vpn-master.service vpn-node.service aimatos-port-hop.service sing-box.service vpn-frontend-standalone.service 2>/dev/null || true
systemctl disable vpn-master.service vpn-node.service aimatos-port-hop.service sing-box.service vpn-frontend-standalone.service 2>/dev/null || true
killall -9 vpn-master vpn-node sing-box aimatos 2>/dev/null || true

# 2. Удаление файлов служб
echo "-> Удаление системных служб Systemd..."
rm -f /etc/systemd/system/vpn-master.service \
      /etc/systemd/system/vpn-node.service \
      /etc/systemd/system/aimatos-port-hop.service \
      /etc/systemd/system/vpn-frontend-standalone.service \
      /etc/systemd/system/sing-box.service
systemctl daemon-reload
systemctl reset-failed

# 3. Удаление рабочих директорий
echo "-> Удаление рабочего каталога /opt/aimatos..."
rm -rf /opt/aimatos
rm -f /usr/local/bin/aimatos
rm -rf /tmp/aimatos* /tmp/go.tar.gz

# 4. Откат сетевых правил iptables и udev
echo "-> Очистка сетевых правил iptables и udev..."
iptables -t nat -F PREROUTING 2>/dev/null || true
rm -f /etc/udev/rules.d/98-ring-buffers.rules /etc/udev/rules.d/60-scheduler.rules
udevadm control --reload-rules 2>/dev/null || true

# 5. Откат sysctl
echo "-> Восстановление конфигурации ядра /etc/sysctl.conf..."
if [ -f /etc/sysctl.conf.bak ]; then
    cp /etc/sysctl.conf.bak /etc/sysctl.conf
    rm -f /etc/sysctl.conf.bak
else
    sed -i '/# VPN Advanced Start/,/# VPN Advanced End/d' /etc/sysctl.conf
    sed -i '/vm.swappiness/d' /etc/sysctl.conf
    sed -i '/vm.vfs_cache_pressure/d' /etc/sysctl.conf
fi
sysctl -p 2>/dev/null || true

# 6. Откат limits.conf и systemd conf
echo "-> Откат лимитов открытых файлов..."
if [ -f /etc/security/limits.conf.bak ]; then
    cp /etc/security/limits.conf.bak /etc/security/limits.conf
    rm -f /etc/security/limits.conf.bak
else
    sed -i '/# VPN Limits Start/,/# VPN Limits End/d' /etc/security/limits.conf
fi
sed -i '/DefaultLimitNOFILE=1048576/d' /etc/systemd/system.conf 2>/dev/null || true
sed -i '/DefaultLimitNOFILE=1048576/d' /etc/systemd/user.conf 2>/dev/null || true

# 7. Откат SSH и logind
echo "-> Восстановление конфигурации SSH..."
if [ -f /etc/ssh/sshd_config.bak ]; then
    cp /etc/ssh/sshd_config.bak /etc/ssh/sshd_config
    rm -f /etc/ssh/sshd_config.bak
else
    sed -i '/^Ciphers chacha20-poly1305/d' /etc/ssh/sshd_config 2>/dev/null || true
    sed -i '/^MACs hmac-sha2-256-etm/d' /etc/ssh/sshd_config 2>/dev/null || true
fi
systemctl restart ssh 2>/dev/null || systemctl restart sshd 2>/dev/null || true

sed -i 's/NAutoVTs=1/#NAutoVTs=6/' /etc/systemd/logind.conf 2>/dev/null || true
systemctl restart systemd-logind 2>/dev/null || true

# 8. Откат Swapfile
if [ -f /swapfile ]; then
    echo "-> Отключение и удаление созданного swapfile..."
    swapoff /swapfile 2>/dev/null || true
    sed -i '\|^/swapfile|d' /etc/fstab 2>/dev/null || true
    rm -f /swapfile
fi

# 9. Очистка репозиториев NodeSource
rm -f /etc/apt/sources.list.d/nodesource* /etc/apt/keyrings/nodesource.gpg /usr/share/keyrings/nodesource.gpg 2>/dev/null || true
apt-get clean

echo "=================================================="
echo "✅ Все следы AimatosPanel успешно удалены!"
echo "=================================================="