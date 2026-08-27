#!/bin/bash
set -eo pipefail

# ==============================================================================
#  AIMATOS PANEL — ULTIMATE CLEAN UNINSTALLER (PURE BASH)
# ==============================================================================

# 1. Принудительно переходим в /root во избежание сбоя getcwd (удаленная директория)
cd /root 2>/dev/null || cd /tmp 2>/dev/null || cd /

# Цветовая палитра Aimatos
CLR_PURPLE="\033[1;35m"
CLR_CYAN="\033[1;36m"
CLR_GREEN="\033[1;32m"
CLR_YELLOW="\033[1;33m"
CLR_RED="\033[1;31m"
CLR_GRAY="\033[0;90m"
CLR_BOLD="\033[1m"
CLR_RESET="\033[0m"

LOCK_FILE="/var/run/aimatos-uninstall.lock"

log_step()    { echo -e " ${CLR_YELLOW}➔${CLR_RESET} ${CLR_BOLD}$1${CLR_RESET}"; }
log_success() { echo -e " ${CLR_GREEN}✓${CLR_RESET} $1"; }
log_info()    { echo -e "    ${CLR_GRAY}└─ $1${CLR_RESET}"; }
log_warn()    { echo -e " ${CLR_YELLOW}⚠${CLR_RESET} $1"; }
log_error()   { echo -e " ${CLR_RED}✗${CLR_RESET} $1"; }

# Проверка прав суперпользователя
if [[ "${EUID:-$(id -u)}" -ne 0 ]]; then
    echo -e "${CLR_RED}❌ Ошибка: Скрипт деинсталляции должен быть запущен с правами root (sudo -i).${CLR_RESET}"
    exit 1
fi

# Функция выполнения всех этапов деинсталляции
perform_full_uninstall() {
    echo ""
    echo -e "${CLR_YELLOW}⚡ Запуск процесса полной очистки AimatosPanel...${CLR_RESET}"
    echo ""

    # 1. Аварийный бэкап базы данных
    log_step "[1/8] Создание аварийной точки восстановления базы данных..."
    if [[ -f /opt/aimatos/vpn-master/panel.db ]]; then
        BACKUP_FILE="/root/aimatos_emergency_backup_$(date +%Y%m%d_%H%M%S).db"
        if command -v sqlite3 >/dev/null 2>&1; then
            sqlite3 /opt/aimatos/vpn-master/panel.db "VACUUM INTO '${BACKUP_FILE}';" 2>/dev/null || cp /opt/aimatos/vpn-master/panel.db "$BACKUP_FILE" 2>/dev/null || true
        else
            cp /opt/aimatos/vpn-master/panel.db "$BACKUP_FILE" 2>/dev/null || true
        fi
        log_success "Аварийный бэкап сохранён: ${CLR_CYAN}${BACKUP_FILE}${CLR_RESET}"
    else
        log_info "База данных не обнаружена (пропуск)"
    fi

    # 2. Остановка и отключение всех служб Systemd
    log_step "[2/8] Остановка системных демонов и фоновых процессов..."
    SERVICES=(
        "vpn-master.service"
        "vpn-node.service"
        "aimatos-port-hop.service"
        "vpn-frontend-standalone.service"
        "sing-box.service"
    )

    for srv in "${SERVICES[@]}"; do
        systemctl stop "$srv" 2>/dev/null || true
        systemctl disable "$srv" 2>/dev/null || true
    done

    # Мягкое завершение процессов, затем принудительное
    killall -SIGTERM vpn-master vpn-node sing-box 2>/dev/null || true
    sleep 1
    killall -9 vpn-master vpn-node sing-box aimatos 2>/dev/null || true

    # Удаление юнит-файлов
    rm -f /etc/systemd/system/vpn-master.service \
          /etc/systemd/system/vpn-node.service \
          /etc/systemd/system/aimatos-port-hop.service \
          /etc/systemd/system/vpn-frontend-standalone.service \
          /etc/systemd/system/sing-box.service
    
    systemctl daemon-reload 2>/dev/null || true
    systemctl reset-failed 2>/dev/null || true
    log_success "Службы удалены, порты (8080, 8085, 8443, 8444, 8445) освобождены"

    # 3. Удаление рабочих каталогов и бинарников
    log_step "[3/8] Удаление директорий /opt/aimatos и бинарных файлов..."
    rm -rf /opt/aimatos /usr/local/bin/aimatos /tmp/aimatos* /tmp/go.tar.gz
    log_success "Рабочие файлы и утилита управления aimatos стёрты"

    # 4. Сброс правил сетевого экрана (iptables / udev)
    log_step "[4/8] Сброс сетевых правил брандмауэра и очередей udev..."
    iptables -t nat -F PREROUTING 2>/dev/null || true
    rm -f /etc/udev/rules.d/98-ring-buffers.rules /etc/udev/rules.d/60-scheduler.rules
    udevadm control --reload-rules 2>/dev/null || true
    log_success "Сетевые правила перенаправления портов очищены"

    # 5. Откат настроек ядра (sysctl) и лимитов дескрипторов (limits.conf)
    log_step "[5/8] Откат параметров ядра и файловых лимитов nofile..."
    if [[ -f /etc/sysctl.conf.bak ]]; then
        cp /etc/sysctl.conf.bak /etc/sysctl.conf && rm -f /etc/sysctl.conf.bak
    else
        sed -i '/# AIMATOS ADVANCED NETWORK START/,/# AIMATOS ADVANCED NETWORK END/d' /etc/sysctl.conf 2>/dev/null || true
        sed -i '/# VPN Advanced Start/,/# VPN Advanced End/d' /etc/sysctl.conf 2>/dev/null || true
        sed -i '/vm.swappiness/d' /etc/sysctl.conf 2>/dev/null || true
        sed -i '/vm.vfs_cache_pressure/d' /etc/sysctl.conf 2>/dev/null || true
    fi
    sysctl -p >/dev/null 2>&1 || true

    if [[ -f /etc/security/limits.conf.bak ]]; then
        cp /etc/security/limits.conf.bak /etc/security/limits.conf && rm -f /etc/security/limits.conf.bak
    else
        sed -i '/# AIMATOS LIMITS START/,/# AIMATOS LIMITS END/d' /etc/security/limits.conf 2>/dev/null || true
        sed -i '/# VPN Limits Start/,/# VPN Limits End/d' /etc/security/limits.conf 2>/dev/null || true
    fi

    sed -i '/DefaultLimitNOFILE=1048576/d' /etc/systemd/system.conf /etc/systemd/user.conf 2>/dev/null || true
    systemctl daemon-reexec 2>/dev/null || true
    log_success "Конфигурация sysctl и limits.conf восстановлена"

    # 6. Восстановление конфигурации SSH и консолей logind
    log_step "[6/8] Восстановление конфигурации SSH и консолей logind..."
    if [[ -f /etc/ssh/sshd_config.bak ]]; then
        cp /etc/ssh/sshd_config.bak /etc/ssh/sshd_config && rm -f /etc/ssh/sshd_config.bak
    else
        sed -i '/^Ciphers chacha20-poly1305/d' /etc/ssh/sshd_config 2>/dev/null || true
        sed -i '/^MACs hmac-sha2-256-etm/d' /etc/ssh/sshd_config 2>/dev/null || true
    fi
    systemctl restart ssh 2>/dev/null || systemctl restart sshd 2>/dev/null || true

    sed -i 's/NAutoVTs=1/#NAutoVTs=6/' /etc/systemd/logind.conf 2>/dev/null || true
    systemctl restart systemd-logind 2>/dev/null || true
    log_success "SSH и системные службы переведены в штатный режим"

    # 7. Отключение Swapfile и ZRAM (если создавались)
    log_step "[7/8] Очистка созданного Swap-пространства и ZRAM..."
    if [[ -f /swapfile ]]; then
        swapoff /swapfile 2>/dev/null || true
        sed -i '\|^/swapfile|d' /etc/fstab 2>/dev/null || true
        rm -f /swapfile
        log_info "Swapfile 2GB отключен и удален"
    fi

    systemctl stop zram-tools.service 2>/dev/null || true
    systemctl disable zram-tools.service 2>/dev/null || true
    rm -f /etc/default/zram-tools.bak
    log_success "Память и виртуальные диски освобождены"

    # 8. Финальная очистка
    log_step "[8/8] Очистка системного кэша и блокировок..."
    rm -f /etc/apt/sources.list.d/nodesource* /etc/apt/keyrings/nodesource.gpg "$LOCK_FILE" 2>/dev/null || true
    apt-get clean 2>/dev/null || true
    log_success "Очистка завершена"

    echo ""
    echo -e "${CLR_GREEN}╔══════════════════════════════════════════════════════════════════════╗${CLR_RESET}"
    echo -e "${CLR_GREEN}║${CLR_RESET}   ${CLR_BOLD}👋  AIMATOS PANEL ПОЛНОСТЬЮ И БЕЗОПАСНО УДАЛЕНА С СЕРВЕРА!        ${CLR_GREEN}║${CLR_RESET}"
    echo -e "${CLR_GREEN}╚══════════════════════════════════════════════════════════════════════╝${CLR_RESET}"
    echo ""
    if [[ -n "${BACKUP_FILE:-}" && -f "$BACKUP_FILE" ]]; then
        echo -e " ${CLR_GRAY}Резервная копия базы данных сохранена в файле:${CLR_RESET}"
        echo -e " ${CLR_CYAN}${BACKUP_FILE}${CLR_RESET}"
        echo ""
    fi
}

# ------------------------------------------------------------------------------
# ТОЧКА ВХОДА: ПРОВЕРКА ФЛАГОВ И ИНТЕРАКТИВНЫЙ ДИАЛОГ
# ------------------------------------------------------------------------------

# Автоматический / тихий режим (--force, -y, --yes, --silent)
if [[ "${1:-}" == "--force" || "${1:-}" == "-y" || "${1:-}" == "--yes" || "${1:-}" == "--silent" ]]; then
    perform_full_uninstall
    exit 0
fi

# Интерактивный экран подтверждения
clear
echo -e "${CLR_RED}╔══════════════════════════════════════════════════════════════════════╗${CLR_RESET}"
echo -e "${CLR_RED}║${CLR_RESET}   ${CLR_YELLOW}⚠️  ДЕИНСТАЛЛЯЦИЯ AIMATOS PANEL                                   ${CLR_RED}║${CLR_RESET}"
echo -e "${CLR_RED}║${CLR_RESET}   ${CLR_GRAY}Полное удаление системных служб, базы данных и бинарников         ${CLR_RED}║${CLR_RESET}"
echo -e "${CLR_RED}╚══════════════════════════════════════════════════════════════════════╝${CLR_RESET}"
echo ""
echo -e " ${CLR_BOLD}Будут выполнены следующие операции:${CLR_RESET}"
echo -e "  • ${CLR_GREEN}Аварийный бэкап базы данных${CLR_RESET} будет сохранён в ${CLR_CYAN}/root/${CLR_RESET}"
echo -e "  • ${CLR_RED}Остановка и удаление служб${CLR_RESET}: vpn-master, vpn-node, sing-box"
echo -e "  • ${CLR_RED}Полное удаление каталога${CLR_RESET}: /opt/aimatos и команды /usr/local/bin/aimatos"
echo -e "  • ${CLR_YELLOW}Откат параметров ядра${CLR_RESET}: sysctl, nofile limits, правила iptables"
echo ""

TARGET_WORD="УДАЛИТЬ"
echo -ne " Для подтверждения введите ${CLR_BOLD}${CLR_RED}${TARGET_WORD}${CLR_RESET} (или любую другую клавишу для отмены): "
read -r USER_INPUT

# Приведение к верхнему регистру для удобства
USER_UPPER=$(echo "$USER_INPUT" | tr '[:lower:]' '[:upper:]')

if [[ "$USER_UPPER" == "$TARGET_WORD" || "$USER_UPPER" == "DELETE" ]]; then
    perform_full_uninstall
else
    echo ""
    echo -e " ${CLR_YELLOW}Отмена деинсталляции. Никакие файлы и службы не были изменены.${CLR_RESET}"
    exit 0
fi