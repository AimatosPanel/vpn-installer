#!/bin/bash
set -eo pipefail

CLR_PURPLE="\033[1;35m"
CLR_GREEN="\033[1;32m"
CLR_RESET="\033[0m"

echo -e "\n${CLR_PURPLE}════ [ Модуль 4: Производительность CPU и Лимиты NOFILE ] ════${CLR_RESET}"

# 1. CPU Governor
echo -e " ${CLR_PURPLE}➔${CLR_RESET} Перевод процессора в режим максимальной производительности..."
apt-get update -y >/dev/null 2>&1 && apt-get install -y cpufrequtils irqbalance >/dev/null 2>&1

for cpu_gov in /sys/devices/system/cpu/cpu*/cpufreq/scaling_governor; do
    [[ -f "$cpu_gov" ]] && echo "performance" > "$cpu_gov" 2>/dev/null || true
done

echo 'GOVERNOR="performance"' > /etc/default/cpufrequtils 2>/dev/null || true
systemctl restart cpufrequtils 2>/dev/null || true

# 2. irqbalance
echo -e " ${CLR_PURPLE}➔${CLR_RESET} Балансировка сетевых прерываний CPU (irqbalance)..."
systemctl enable irqbalance >/dev/null 2>&1
systemctl restart irqbalance >/dev/null 2>&1

# 3. Лимиты файлов (NOFILE)
echo -e " ${CLR_PURPLE}➔${CLR_RESET} Увеличение системных лимитов дескрипторов (1 048 576)..."
LIMITS_CONF="/etc/security/limits.conf"
[[ ! -f "${LIMITS_CONF}.bak" ]] && cp "$LIMITS_CONF" "${LIMITS_CONF}.bak"

sed -i '/# AIMATOS LIMITS START/,/# AIMATOS LIMITS END/d' "$LIMITS_CONF"

cat <<'EOF' >> "$LIMITS_CONF"
# AIMATOS LIMITS START
* soft nofile 1048576
* hard nofile 1048576
root soft nofile 1048576
root hard nofile 1048576
# AIMATOS LIMITS END
EOF

sed -i '/DefaultLimitNOFILE/d' /etc/systemd/system.conf /etc/systemd/user.conf 2>/dev/null || true
echo "DefaultLimitNOFILE=1048576" >> /etc/systemd/system.conf
echo "DefaultLimitNOFILE=1048576" >> /etc/systemd/user.conf
systemctl daemon-reexec 2>/dev/null || true

# 4. I/O планировщик
cat <<'EOF' > /etc/udev/rules.d/60-scheduler.rules
ACTION=="add|change", KERNEL=="sd[a-z]|vd[a-z]|nvme[0-9]n[0-9]", ATTR{queue/scheduler}="none"
EOF

echo -e " ${CLR_GREEN}✓ Модуль 4 успешно применён!${CLR_RESET}"