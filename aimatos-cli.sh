#!/bin/bash


RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
NC='\033[0m'


if [ "$EUID" -ne 0 ]; then
    echo -e "${RED}Ошибка: Пожалуйста, запустите CLI утилиту от имени суперпользователя (root).${NC}"
    exit 1
fi

INSTALL_DIR="/opt/aimatos"


get_sys_info() {
    CPU_USAGE=$(top -bn1 | grep "Cpu(s)" | sed "s/.*, *\([0-9.]*\)%* id.*/\1/" | awk '{print 100 - $1}')
    RAM_TOTAL=$(free -m | awk '/Mem:/ { print $2 }')
    RAM_USED=$(free -m | awk '/Mem:/ { print $3 }')
    RAM_PCT=$(awk "BEGIN {print ($RAM_USED/$RAM_TOTAL)*100}")
    UPTIME=$(uptime -p)
}


get_service_status() {
    MASTER_STATE="Not Installed"
    NODE_STATE="Not Installed"
    FRONTEND_STATE="Not Installed"

    if systemctl list-unit-files | grep -q "vpn-master.service"; then
        if systemctl is-active --quiet vpn-master.service; then
            MASTER_STATE="${GREEN}Active (Running)${NC}"
        else
            MASTER_STATE="${RED}Inactive (Stopped)${NC}"
        fi
    fi

    if systemctl list-unit-files | grep -q "vpn-node.service"; then
        if systemctl is-active --quiet vpn-node.service; then
            NODE_STATE="${GREEN}Active (Running)${NC}"
        else
            NODE_STATE="${RED}Inactive (Stopped)${NC}"
        fi
    fi

    if systemctl list-unit-files | grep -q "vpn-frontend-standalone.service"; then
        if systemctl is-active --quiet vpn-frontend-standalone.service; then
            FRONTEND_STATE="${GREEN}Active (Running)${NC}"
        else
            FRONTEND_STATE="${RED}Inactive (Stopped)${NC}"
        fi
    fi
}


show_status() {
    clear
    get_sys_info
    get_service_status
    echo -e "${CYAN}===================================================${NC}"
    echo -e "          ⚙️  AimatosPanel Мониторинг Системы        "
    echo -e "${CYAN}===================================================${NC}"
    echo -e " Аптайм сервера:      ${YELLOW}$UPTIME${NC}"
    echo -e " Загрузка процессора: ${YELLOW}${CPU_USAGE}%${NC}"
    echo -e " Использование ОЗУ:   ${YELLOW}${RAM_USED}MB / ${RAM_TOTAL}MB (${RAM_PCT%.*}%)${NC}"
    echo -e "${CYAN}---------------------------------------------------${NC}"
    echo -e " vpn-master service:  $MASTER_STATE"
    echo -e " vpn-node service:    $NODE_STATE"
    echo -e " vpn-frontend (SA):   $FRONTEND_STATE"
    
    if [ -f "$INSTALL_DIR/vpn-node/sing-box" ]; then
        if pidof sing-box >/dev/null; then
            echo -e " Ядро Sing-Box:       ${GREEN}Online (Active)${NC}"
        else
            echo -e " Ядро Sing-Box:       ${RED}Offline (Stopped)${NC}"
        fi
    fi
    echo -e "${CYAN}===================================================${NC}"
    read -p "Нажмите Enter для возврата в меню..." dummy
}


show_access_link() {
    clear
    echo -e "${CYAN}===================================================${NC}"
    echo -e "          🔗 Ссылки доступа к AimatosPanel         "
    echo -e "${CYAN}===================================================${NC}"
    LOCAL_IP=$(curl -s ifconfig.me || echo "127.0.0.1")
    

    if [ -f "$INSTALL_DIR/vpn-master/panel.db" ]; then
        API_KEY=$(sqlite3 "$INSTALL_DIR/vpn-master/panel.db" "SELECT value FROM settings WHERE key='api_key';" 2>/dev/null)
    fi
    
    if [ -z "$API_KEY" ]; then
        API_KEY="Не найден (Возможно, установлен удаленный мастер)"
    fi

    echo -e " Внешний IP сервера:  ${GREEN}$LOCAL_IP${NC}"
    echo -e " API Ключ:            ${GREEN}$API_KEY${NC}"
    echo -e "${CYAN}---------------------------------------------------${NC}"
    echo -e " 🌍 Ссылка на встроенную админ-панель:"
    echo -e "   ${YELLOW}http://$LOCAL_IP:8080?X-API-Key=$API_KEY${NC}"
    
    if systemctl list-unit-files | grep -q "vpn-frontend-standalone.service"; then
        echo -e "\n 🌍 Ссылка на автономный фронтенд (Standalone):"
        echo -e "   ${YELLOW}http://$LOCAL_IP:3000?X-API-Key=$API_KEY${NC}"
    fi
    echo -e "${CYAN}===================================================${NC}"
    read -p "Нажмите Enter для возврата в меню..." dummy
}


manage_settings() {
    clear
    echo -e "${CYAN}===================================================${NC}"
    echo -e "          ⚙️  Настройки и конфигурации портов       "
    echo -e "${CYAN}===================================================${NC}"
    if [ ! -f "$INSTALL_DIR/vpn-master/panel.db" ]; then
        echo -e "${RED}Мастер БД не найдена на этом сервере.${NC}"
        read -p "Нажмите Enter для возврата..." dummy
        return
    fi

    echo "Текущие порты в конфигурации:"
    VLESS_PORT=$(sqlite3 "$INSTALL_DIR/vpn-master/panel.db" "SELECT value FROM settings WHERE key='vless_port';")
    HYSTERIA_PORT=$(sqlite3 "$INSTALL_DIR/vpn-master/panel.db" "SELECT value FROM settings WHERE key='hysteria_port';")
    TUIC_PORT=$(sqlite3 "$INSTALL_DIR/vpn-master/panel.db" "SELECT value FROM settings WHERE key='tuic_port';")
    NAIVE_PORT=$(sqlite3 "$INSTALL_DIR/vpn-master/panel.db" "SELECT value FROM settings WHERE key='naive_port';")

    echo -e " 1) VLESS Port:      ${YELLOW}$VLESS_PORT${NC}"
    echo -e " 2) Hysteria 2 Port: ${YELLOW}$HYSTERIA_PORT${NC}"
    echo -e " 3) TUIC Port:       ${YELLOW}$TUIC_PORT${NC}"
    echo -e " 4) NaiveProxy Port: ${YELLOW}$NAIVE_PORT${NC}"
    echo " 0) Назад"
    echo -e "${CYAN}---------------------------------------------------${NC}"
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
    echo -e "${CYAN}===================================================${NC}"
    echo -e "          🔄 Перезапуск активных служб панели       "
    echo -e "${CYAN}===================================================${NC}"
    
    if systemctl list-unit-files | grep -q "vpn-master.service"; then
        echo -e "Перезапуск vpn-master..."
        systemctl restart vpn-master.service
    fi

    if systemctl list-unit-files | grep -q "vpn-node.service"; then
        echo -e "Перезапуск vpn-node..."
        systemctl restart vpn-node.service
    fi

    if systemctl list-unit-files | grep -q "vpn-frontend-standalone.service"; then
        echo -e "Перезапуск vpn-frontend-standalone..."
        systemctl restart vpn-frontend-standalone.service
    fi

    echo -e "${GREEN}Все установленные службы успешно перезапущены!${NC}"
    read -p "Нажмите Enter для возврата..." dummy
}


uninstall_panel() {
    clear
    echo -e "${RED}===================================================${NC}"
    echo -e "          ⚠️  ВНИМАНИЕ! ПОЛНОЕ УДАЛЕНИЕ СИСТЕМЫ    "
    echo -e "${RED}===================================================${NC}"
    echo -e "Все базы данных, конфигурации, ключи пользователей и"
    echo -e "скомпилированные бинарные файлы будут безвозвратно удалены."
    echo -e "${RED}===================================================${NC}"
    read -p "Вы уверены, что хотите продолжить? [y/N]: " confirm_un

    if [[ "$confirm_un" =~ ^[Yy]$ ]]; then
        echo -e "\n${BLUE}Остановка системных служб...${NC}"
        systemctl stop vpn-master.service 2>/dev/null || true
        systemctl stop vpn-node.service 2>/dev/null || true
        systemctl stop vpn-frontend-standalone.service 2>/dev/null || true

        echo -e "${BLUE}Отключение автозапуска служб...${NC}"
        systemctl disable vpn-master.service 2>/dev/null || true
        systemctl disable vpn-node.service 2>/dev/null || true
        systemctl disable vpn-frontend-standalone.service 2>/dev/null || true

        echo -e "${BLUE}Удаление конфигураций systemd...${NC}"
        rm -f /etc/systemd/system/vpn-master.service
        rm -f /etc/systemd/system/vpn-node.service
        rm -f /etc/systemd/system/vpn-frontend-standalone.service
        systemctl daemon-reload

        echo -e "${BLUE}Очистка директории установки и исполняемых файлов...${NC}"
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
    echo -e "${PURPLE}===================================================${NC}"
    echo -e "           🛡️  AimatosPanel CLI Консоль             "
    echo -e "${PURPLE}===================================================${NC}"
    echo " 1) Статус системы и метрики"
    echo " 2) Ссылки доступа к Админ-панели"
    echo " 3) Конфигурация портов и настроек"
    echo " 4) Перезапустить службы"
    echo " 5) Полное удаление системы"
    echo " 0) Выход"
    echo -e "${PURPLE}===================================================${NC}"
    read -p "Выберите действие: " menu_choice

    case "$menu_choice" in
        1) show_status ;;
        2) show_access_link ;;
        3) manage_settings ;;
        4) restart_services ;;
        5) uninstall_panel ;;
        0) clear; exit 0 ;;
        *) echo -e "${RED}Неверный ввод. Попробуйте еще раз.${NC}"; sleep 1 ;;
    esac
done