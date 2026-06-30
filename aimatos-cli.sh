#!/bin/bash

RED='\033[38;5;196m'
GREEN='\033[38;5;46m'
YELLOW='\033[38;5;220m'
BLUE='\033[38;5;39m'
PURPLE='\033[38;5;129m'
CYAN='\033[38;5;51m'
GRAY='\033[38;5;244m'
NC='\033[0m'

if [ "$EUID" -ne 0 ]; then
    echo -e "${RED}❌ Ошибка: Запустите CLI от имени суперпользователя (root).${NC}"
    exit 1
fi

INSTALL_DIR="/opt/aimatos"

draw_bar() {
    local percentage=$1
    local width=15
    local filled=$(( percentage * width / 100 ))
    local empty=$(( width - filled ))
    local bar=""
    for ((i=0; i<filled; i++)); do bar="${bar}█"; done
    for ((i=0; i<empty; i++)); do bar="${bar}░"; done
    if [ "$percentage" -lt 50 ]; then
        echo -e "${GREEN}[${bar}] ${percentage}%${NC}"
    elif [ "$percentage" -lt 80 ]; then
        echo -e "${YELLOW}[${bar}] ${percentage}%${NC}"
    else
        echo -e "${RED}[${bar}] ${percentage}%${NC}"
    fi
}

get_sys_info() {
    CPU_USAGE=$(top -bn1 | grep "Cpu(s)" | sed "s/.*, *\([0-9.]*\)%* id.*/\1/" | awk '{print int(100 - $1)}')
    RAM_TOTAL=$(free -m | awk '/Mem:/ { print $2 }')
    RAM_USED=$(free -m | awk '/Mem:/ { print $3 }')
    RAM_PCT=$(awk "BEGIN {print int(($RAM_USED/$RAM_TOTAL)*100)}")
    UPTIME=$(uptime -p | sed 's/up //')
}

get_service_status() {
    get_status_label() {
        if systemctl list-unit-files | grep -q "$1"; then
            if systemctl is-active --quiet "$1"; then
                echo -e "${GREEN}● Active${NC}"
            else
                echo -e "${RED}○ Stopped${NC}"
            fi
        else
            echo -e "${GRAY}Not Installed${NC}"
        fi
    }
    MASTER_STATE=$(get_status_label "vpn-master.service")
    NODE_STATE=$(get_status_label "vpn-node.service")
    FRONTEND_STATE=$(get_status_label "vpn-frontend-standalone.service")
}

show_status() {
    clear
    get_sys_info
    get_service_status
    echo -e "${PURPLE}┌────────────────────────────────────────────────────────┐${NC}"
    echo -e "${PURPLE}│${NC}               🛸  ${CYAN}AIMATOS PANEL METRICS${NC}               ${PURPLE}│${NC}"
    echo -e "${PURPLE}├────────────────────────────────────────────────────────┤${NC}"
    echo -e "${PURPLE}│${NC}  ${GRAY}System Uptime:${NC}   %-36s ${PURPLE}│${NC}" "$UPTIME"
    echo -e "${PURPLE}│${NC}  ${GRAY}CPU Utilization:${NC} %-36s ${PURPLE}│${NC}" "$(draw_bar $CPU_USAGE)"
    echo -e "${PURPLE}│${NC}  ${GRAY}RAM Memory:${NC}      %-36s ${PURPLE}│${NC}" "$(draw_bar $RAM_PCT) (${RAM_USED}MB/${RAM_TOTAL}MB)"
    echo -e "${PURPLE}├────────────────────────────────────────────────────────┤${NC}"
    echo -e "${PURPLE}│${NC}                   ${CYAN}SERVICES STATUS${NC}                      ${PURPLE}│${NC}"
    echo -e "${PURPLE}├────────────────────────────────────────────────────────┤${NC}"
    echo -e "${PURPLE}│${NC}  ▸ Master Service:   %-38s ${PURPLE}│${NC}" "$MASTER_STATE"
    echo -e "${PURPLE}│${NC}  ▸ Node Service:     %-38s ${PURPLE}│${NC}" "$NODE_STATE"
    echo -e "${PURPLE}│${NC}  ▸ Standalone Web:   %-38s ${PURPLE}│${NC}" "$FRONTEND_STATE"
    echo -e "${PURPLE}└────────────────────────────────────────────────────────┘${NC}"
    echo ""
    read -p "Нажмите Enter для возврата в меню..." dummy
}

show_access_link() {
    clear
    echo -e "${CYAN}┌────────────────────────────────────────────────────────┐${NC}"
    echo -e "${CYAN}│${NC}               🔗  AIMATOS ACCESS LINKS                 ${CYAN}│${NC}"
    echo -e "${CYAN}├────────────────────────────────────────────────────────┤${NC}"
    LOCAL_IP=$(curl -s ifconfig.me || echo "127.0.0.1")
    if [ -f "$INSTALL_DIR/vpn-master/panel.db" ]; then
        API_KEY=$(sqlite3 "$INSTALL_DIR/vpn-master/panel.db" "SELECT value FROM settings WHERE key='api_key';" 2>/dev/null)
    fi
    if [ -z "$API_KEY" ]; then
        API_KEY="Not found"
    fi
    echo -e "${CYAN}│${NC}  ▸ Server IP:       ${GREEN}$LOCAL_IP${NC}"
    echo -e "${CYAN}│${NC}  ▸ API Key:         ${GREEN}$API_KEY${NC}"
    echo -e "${CYAN}├────────────────────────────────────────────────────────┤${NC}"
    echo -e "${CYAN}│${NC}  ▸ Admin Web Panel Link:"
    echo -e "${CYAN}│${NC}    ${YELLOW}http://$LOCAL_IP:8080?X-API-Key=$API_KEY${NC}"
    if systemctl list-unit-files | grep -q "vpn-frontend-standalone.service"; then
        echo -e "${CYAN}│${NC}"
        echo -e "${CYAN}│${NC}  ▸ Standalone Frontend Link:"
        echo -e "${CYAN}│${NC}    ${YELLOW}http://$LOCAL_IP:3000?X-API-Key=$API_KEY${NC}"
    fi
    echo -e "${CYAN}└────────────────────────────────────────────────────────┘${NC}"
    echo ""
    read -p "Нажмите Enter для возврата в меню..." dummy
}

manage_settings() {
    clear
    echo -e "${CYAN}┌────────────────────────────────────────────────────────┐${NC}"
    echo -e "${CYAN}│${NC}               ⚙️  PORT CONFIGURATION                     ${CYAN}│${NC}"
    echo -e "${CYAN}├────────────────────────────────────────────────────────┤${NC}"
    if [ ! -f "$INSTALL_DIR/vpn-master/panel.db" ]; then
        echo -e "${RED}Мастер БД не найдена на этом сервере.${NC}"
        read -p "Нажмите Enter для возврата..." dummy
        return
    fi
    VLESS_PORT=$(sqlite3 "$INSTALL_DIR/vpn-master/panel.db" "SELECT value FROM settings WHERE key='vless_port';")
    HYSTERIA_PORT=$(sqlite3 "$INSTALL_DIR/vpn-master/panel.db" "SELECT value FROM settings WHERE key='hysteria_port';")
    TUIC_PORT=$(sqlite3 "$INSTALL_DIR/vpn-master/panel.db" "SELECT value FROM settings WHERE key='tuic_port';")
    NAIVE_PORT=$(sqlite3 "$INSTALL_DIR/vpn-master/panel.db" "SELECT value FROM settings WHERE key='naive_port';")
    echo -e "${CYAN}│${NC}  ${YELLOW}1)${NC} VLESS Port:      ${GREEN}$VLESS_PORT${NC}"
    echo -e "${CYAN}│${NC}  ${YELLOW}2)${NC} Hysteria 2 Port: ${GREEN}$HYSTERIA_PORT${NC}"
    echo -e "${CYAN}│${NC}  ${YELLOW}3)${NC} TUIC Port:       ${GREEN}$TUIC_PORT${NC}"
    echo -e "${CYAN}│${NC}  ${YELLOW}4)${NC} NaiveProxy Port: ${GREEN}$NAIVE_PORT${NC}"
    echo -e "${CYAN}│${NC}  ${YELLOW}0)${NC} Назад"
    echo -e "${CYAN}└────────────────────────────────────────────────────────┘${NC}"
    read -p "Выберите порт для изменения [1-4]: " port_choice
    case "$port_choice" in
        1)
            read -p "Введите новый VLESS порт: " new_port
            sqlite3 "$INSTALL_DIR/vpn-master/panel.db" "UPDATE settings SET value='$new_port' WHERE key='vless_port';"
            ufw allow "$new_port"/tcp
            ;;
        2)
            read -p "Введите новый Hysteria 2 порт: " new_port
            sqlite3 "$INSTALL_DIR/vpn-master/panel.db" "UPDATE settings SET value='$new_port' WHERE key='hysteria_port';"
            ufw allow "$new_port"/tcp
            ufw allow "$new_port"/udp
            ;;
        3)
            read -p "Введите новый TUIC порт: " new_port
            sqlite3 "$INSTALL_DIR/vpn-master/panel.db" "UPDATE settings SET value='$new_port' WHERE key='tuic_port';"
            ufw allow "$new_port"/udp
            ;;
        4)
            read -p "Введите новый NaiveProxy порт: " new_port
            sqlite3 "$INSTALL_DIR/vpn-master/panel.db" "UPDATE settings SET value='$new_port' WHERE key='naive_port';"
            ufw allow "$new_port"/tcp
            ;;
    esac
    systemctl restart vpn-master.service 2>/dev/null || true
    systemctl restart vpn-node.service 2>/dev/null || true
}

restart_services() {
    clear
    echo -e "${CYAN}┌────────────────────────────────────────────────────────┐${NC}"
    echo -e "${CYAN}│${NC}               🔄 RESTARTING SERVICES                    ${CYAN}│${NC}"
    echo -e "${CYAN}├────────────────────────────────────────────────────────┤${NC}"
    if systemctl list-unit-files | grep -q "vpn-master.service"; then
        echo -e "${CYAN}│${NC}  Перезапуск vpn-master..."
        systemctl restart vpn-master.service
    fi
    if systemctl list-unit-files | grep -q "vpn-node.service"; then
        echo -e "${CYAN}│${NC}  Перезапуск vpn-node..."
        systemctl restart vpn-node.service
    fi
    if systemctl list-unit-files | grep -q "vpn-frontend-standalone.service"; then
        echo -e "${CYAN}│${NC}  Перезапуск vpn-frontend-standalone..."
        systemctl restart vpn-frontend-standalone.service
    fi
    echo -e "${CYAN}├────────────────────────────────────────────────────────┤${NC}"
    echo -e "${CYAN}│${NC}  ${GREEN}Все установленные службы успешно перезапущены!${NC}"
    echo -e "${CYAN}└────────────────────────────────────────────────────────┘${NC}"
    echo ""
    read -p "Нажмите Enter для возврата..." dummy
}

uninstall_panel() {
    clear
    echo -e "${RED}┌────────────────────────────────────────────────────────┐${NC}"
    echo -e "${RED}│${NC}               ⚠️  ВНИМАНИЕ! ПОЛНОЕ УДАЛЕНИЕ СИСТЕМЫ     ${RED}│${NC}"
    echo -e "${RED}├────────────────────────────────────────────────────────┤${NC}"
    echo -e "${RED}│${NC} Все базы данных, конфигурации, ключи пользователей и     ${RED}│${NC}"
    echo -e "${RED}│${NC} скомпилированные бинарные файлы будут безвозвратно       ${RED}│${NC}"
    echo -e "${RED}│${NC} удалены с данного сервера.                               ${RED}│${NC}"
    echo -e "${RED}└────────────────────────────────────────────────────────┘${NC}"
    read -p "Вы уверены, что хотите продолжить? [y/N]: " confirm_un
    if [[ "$confirm_un" =~ ^[Yy]$ ]]; then
        systemctl stop vpn-master.service 2>/dev/null || true
        systemctl stop vpn-node.service 2>/dev/null || true
        systemctl stop vpn-frontend-standalone.service 2>/dev/null || true
        systemctl disable vpn-master.service 2>/dev/null || true
        systemctl disable vpn-node.service 2>/dev/null || true
        systemctl disable vpn-frontend-standalone.service 2>/dev/null || true
        rm -f /etc/systemd/system/vpn-master.service
        rm -f /etc/systemd/system/vpn-node.service
        rm -f /etc/systemd/system/vpn-frontend-standalone.service
        systemctl daemon-reload
        rm -rf "$INSTALL_DIR"
        rm -f /usr/local/bin/aimatos
        echo -e "${GREEN}AimatosPanel успешно удалена со всеми компонентами.${NC}"
        exit 0
    else
        echo -e "Удаление отменено."
        sleep 1
    fi
}

while true; do
    clear
    echo -e "${CYAN}┌────────────────────────────────────────────────────────┐${NC}"
    echo -e "${CYAN}│${NC}                🔮 ${PURPLE}AIMATOS VPN CONTROL${NC}                 ${CYAN}│${NC}"
    echo -e "${CYAN}├────────────────────────────────────────────────────────┤${NC}"
    echo -e "${CYAN}│${NC}  ${YELLOW}1)${NC} Показать метрики и статус системы                  ${CYAN}│${NC}"
    echo -e "${CYAN}│${NC}  ${YELLOW}2)${NC} Получить ссылки доступа к админ-панели             ${CYAN}│${NC}"
    echo -e "${CYAN}│${NC}  ${YELLOW}3)${NC} Изменение портов подключения (сеть)                ${CYAN}│${NC}"
    echo -e "${CYAN}│${NC}  ${YELLOW}4)${NC} Быстрый перезапуск служб                           ${CYAN}│${NC}"
    echo -e "${CYAN}│${NC}  ${RED}5)${NC} Полное удаление ПО с сервера                       ${CYAN}│${NC}"
    echo -e "${CYAN}│${NC}  ${GRAY}0) Выйти из CLI${NC}                                     ${CYAN}│${NC}"
    echo -e "${CYAN}└────────────────────────────────────────────────────────┘${NC}"
    read -p "Выберите действие [0-5]: " menu_choice
    case "$menu_choice" in
        1) show_status ;;
        2) show_access_link ;;
        3) manage_settings ;;
        4) restart_services ;;
        5) uninstall_panel ;;
        0) clear; exit 0 ;;
        *) echo -e "${RED}Неверный выбор. Пожалуйста, повторите.${NC}"; sleep 1 ;;
    esac
done
