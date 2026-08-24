#!/bin/bash
if [ "$EUID" -ne 0 ]; then
  echo "Запустите от имени root."
  exit 1
fi

echo "=== Модуль 4: Оптимизация Процессора и Лимитов ==="

echo "-> Настройка CPU Governor..."
apt update && apt install -y cpufrequtils

for cpu_gov in /sys/devices/system/cpu/cpu*/cpufreq/scaling_governor; do
    if [ -f "$cpu_gov" ]; then
        echo "performance" > "$cpu_gov" || true
    fi
done

echo 'GOVERNOR="performance"' > /etc/default/cpufrequtils
systemctl restart cpufrequtils 2>/dev/null || true

echo "-> Установка и запуск irqbalance..."
apt install -y irqbalance
systemctl enable irqbalance
systemctl restart irqbalance

echo "-> Настройка лимитов открытых файлов..."
LIMITS_CONF="/etc/security/limits.conf"
cp "$LIMITS_CONF" "${LIMITS_CONF}.bak"
sed -i '/# VPN Limits/,$d' "$LIMITS_CONF"

cat <<EOF >> "$LIMITS_CONF"
# VPN Limits Start
* soft nofile 1048576
* hard nofile 1048576
root soft nofile 1048576
root hard nofile 1048576
# VPN Limits End
EOF

echo "DefaultLimitNOFILE=1048576" >> /etc/systemd/system.conf
echo "DefaultLimitNOFILE=1048576" >> /etc/systemd/user.conf
systemctl daemon-reexec

echo "-> Тюнинг I/O планировщика..."
cat <<EOF > /etc/udev/rules.d/60-scheduler.rules
ACTION=="add|change", KERNEL=="sd[a-z]|vd[a-z]", ATTR{queue/scheduler}="none"
EOF

echo "=== Скрипт 4 выполнен ==="