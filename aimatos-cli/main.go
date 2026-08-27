package main

import (
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	_ "modernc.org/sqlite"
)

const (
	DBPath           = "/opt/aimatos/vpn-master/panel.db"
	BackupsDir       = "/opt/aimatos/backups"
	MasterServiceFile = "/etc/systemd/system/vpn-master.service"
)

// Цветовая схема Aimatos Cyberpunk
var (
	accentColor  = lipgloss.Color("99")  // Фиолетовый
	pinkColor    = lipgloss.Color("205") // Розовый
	grayColor    = lipgloss.Color("244")
	successColor = lipgloss.Color("46")  // Зеленый
	failColor    = lipgloss.Color("196") // Красный
	amberColor   = lipgloss.Color("214") // Оранжевый

	titleStyle    = lipgloss.NewStyle().Foreground(pinkColor).Bold(true).Align(lipgloss.Center)
	subtitleStyle = lipgloss.NewStyle().Foreground(grayColor).Align(lipgloss.Center)
	windowStyle   = lipgloss.NewStyle().Border(lipgloss.DoubleBorder()).BorderForeground(accentColor).Padding(1, 3).Width(74)
	successStyle  = lipgloss.NewStyle().Foreground(successColor).Bold(true)
	failStyle     = lipgloss.NewStyle().Foreground(failColor).Bold(true)
	focusStyle    = lipgloss.NewStyle().Foreground(accentColor).Bold(true)
	grayStyle     = lipgloss.NewStyle().Foreground(grayColor)
	helpStyle     = lipgloss.NewStyle().Foreground(grayColor).Align(lipgloss.Center)
)

type menuState int

const (
	stateMain menuState = iota
	stateStatus
	stateLinks
	stateNodeCluster
	stateConfigMenu
	stateConfigEdit
	stateToolsMenu
	stateBackupList
)

type BackupItem struct {
	Name string
	Path string
	Size string
	Time string
}

type extProcessFinishedMsg struct {
	action string
	err    error
}

type model struct {
	state          menuState
	mainChoice     int
	configChoice   int
	toolsChoice    int
	cursorIndex    int
	input          textinput.Model
	spinner        spinner.Model
	db             *sql.DB
	termWidth      int
	termHeight     int
	outputMsg      string
	apiKey         string
	serverIP       string
	webPort        string
	nodePort       string
	backups        []BackupItem
	currentSetting string
	settingLabel   string
}

// -------------------------------------------------------------
// ВСПОМОГАТЕЛЬНЫЕ ФУНКЦИИ
// -------------------------------------------------------------

func formatBytes(bytes int64) string {
	if bytes <= 0 {
		return "0 B"
	}
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func getPublicIP() string {
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("https://api.ipify.org")
	if err != nil {
		return "127.0.0.1"
	}
	defer resp.Body.Close()
	ip, err := io.ReadAll(resp.Body)
	if err != nil {
		return "127.0.0.1"
	}
	return string(ip)
}

func getMasterServicePort() string {
	data, err := os.ReadFile(MasterServiceFile)
	if err != nil {
		return "8080"
	}
	re := regexp.MustCompile(`Environment=PORT=(\d+)`)
	match := re.FindStringSubmatch(string(data))
	if len(match) > 1 {
		return match[1]
	}
	return "8080"
}

func setMasterServicePort(port string) error {
	data, err := os.ReadFile(MasterServiceFile)
	if err != nil {
		return err
	}
	re := regexp.MustCompile(`Environment=PORT=\d+`)
	newContent := re.ReplaceAllString(string(data), fmt.Sprintf("Environment=PORT=%s", port))
	return os.WriteFile(MasterServiceFile, []byte(newContent), 0644)
}

func (m *model) getSetting(key, fallback string) string {
	if m.db == nil {
		return fallback
	}
	var val string
	err := m.db.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&val)
	if err != nil || val == "" {
		return fallback
	}
	return val
}

func (m *model) setSetting(key, val string) error {
	if m.db == nil {
		return fmt.Errorf("нет связи с БД")
	}
	_, err := m.db.Exec("INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)", key, val)
	return err
}

func (m *model) reloadBackups() {
	m.backups = nil
	files, err := filepath.Glob(filepath.Join(BackupsDir, "panel_backup_*.db"))
	if err != nil {
		return
	}
	for _, f := range files {
		info, err := os.Stat(f)
		if err == nil {
			m.backups = append(m.backups, BackupItem{
				Name: filepath.Base(f),
				Path: f,
				Size: formatBytes(info.Size()),
				Time: info.ModTime().Format("2006-01-02 15:04:05"),
			})
		}
	}
}

// -------------------------------------------------------------
// ИНИЦИАЛИЗАЦИЯ И ОБРАБОТКА СОБЫТИЙ
// -------------------------------------------------------------

func initialModel() model {
	db, err := sql.Open("sqlite", DBPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		fmt.Printf("Ошибка подключения к БД: %v\n", err)
		os.Exit(1)
	}

	ti := textinput.New()
	ti.Placeholder = "Новое значение..."
	ti.CharLimit = 64
	ti.Width = 30

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(accentColor)

	m := model{
		state:       stateMain,
		mainChoice:  0,
		input:       ti,
		spinner:     s,
		db:          db,
		serverIP:    getPublicIP(),
		webPort:     getMasterServicePort(),
		nodePort:    "8085",
		cursorIndex: 0,
	}

	m.apiKey = m.getSetting("api_key", "SuperSecretAdminKey123")
	m.nodePort = m.getSetting("node_port", "8085")
	return m
}

func (m model) Init() tea.Cmd {
	return tea.Batch(tea.EnterAltScreen, m.spinner.Tick)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.termWidth = msg.Width
		m.termHeight = msg.Height
		return m, nil

	case extProcessFinishedMsg:
		m.state = stateMain
		switch msg.action {
		case "logs":
			m.outputMsg = "Просмотр системного журнала завершён."
		case "update":
			if msg.err != nil {
				m.outputMsg = "Процесс обновления прерван."
			} else {
				m.outputMsg = "Обновление AimatosPanel успешно завершено!"
			}
			m.webPort = getMasterServicePort()
			m.apiKey = m.getSetting("api_key", "SuperSecretAdminKey123")
		case "uninstall":
			// Если база данных стёрта — система удалена, завершаем работу
			if _, err := os.Stat(DBPath); os.IsNotExist(err) {
				if m.db != nil {
					m.db.Close()
				}
				clearCmd := exec.Command("clear")
				clearCmd.Stdout = os.Stdout
				_ = clearCmd.Run()
				fmt.Println(successStyle.Render("👋 AimatosPanel успешно и полностью удалена с сервера. До свидания!"))
				os.Exit(0)
			}
			m.outputMsg = "Деинсталляция отменена пользователем."
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			if m.db != nil {
				m.db.Close()
			}
			return m, tea.Quit
		case "q":
			if m.state == stateMain {
				if m.db != nil {
					m.db.Close()
				}
				return m, tea.Quit
			}
			m.state = stateMain
			m.outputMsg = ""
			return m, nil
		}

		switch m.state {
		case stateMain:
			switch msg.String() {
			case "up", "k":
				if m.mainChoice > 0 {
					m.mainChoice--
				}
			case "down", "j":
				if m.mainChoice < 7 {
					m.mainChoice++
				}
			case "enter":
				cmd := m.handleMainMenu()
				if cmd != nil {
					return m, cmd
				}
			}

		case stateConfigMenu:
			switch msg.String() {
			case "up", "k":
				if m.configChoice > 0 {
					m.configChoice--
				}
			case "down", "j":
				if m.configChoice < 6 {
					m.configChoice++
				}
			case "enter":
				m.handleConfigMenu()
			case "esc":
				m.state = stateMain
			}

		case stateConfigEdit:
			switch msg.String() {
			case "enter":
				m.saveConfigValue()
			case "esc":
				m.state = stateConfigMenu
			}
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd

		case stateToolsMenu:
			switch msg.String() {
			case "up", "k":
				if m.toolsChoice > 0 {
					m.toolsChoice--
				}
			case "down", "j":
				if m.toolsChoice < 4 {
					m.toolsChoice++
				}
			case "enter":
				m.handleToolsMenu()
			case "esc":
				m.state = stateMain
			}

		case stateBackupList:
			switch msg.String() {
			case "up", "k":
				if m.cursorIndex > 0 {
					m.cursorIndex--
				}
			case "down", "j":
				if m.cursorIndex < len(m.backups)-1 {
					m.cursorIndex++
				}
			case "enter":
				if len(m.backups) > 0 {
					target := m.backups[m.cursorIndex]
					m.restoreBackup(target.Path)
				}
			case "esc":
				m.state = stateToolsMenu
			}

		default:
			if msg.String() == "enter" || msg.String() == "esc" {
				m.state = stateMain
				m.outputMsg = ""
			}
		}
	}

	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)
	return m, cmd
}

// -------------------------------------------------------------
// ОБРАБОТКА ДЕЙСТВИЙ МЕНЮ
// -------------------------------------------------------------

func (m *model) handleMainMenu() tea.Cmd {
	m.outputMsg = ""
	switch m.mainChoice {
	case 0:
		m.state = stateStatus
	case 1:
		m.state = stateLinks
		m.apiKey = m.getSetting("api_key", "SuperSecretAdminKey123")
		m.webPort = getMasterServicePort()
	case 2:
		m.state = stateNodeCluster
		m.apiKey = m.getSetting("api_key", "SuperSecretAdminKey123")
		m.nodePort = m.getSetting("node_port", "8085")
	case 3:
		m.state = stateConfigMenu
		m.configChoice = 0
	case 4:
		// Реальный стриминг системного журнала
		c := exec.Command("journalctl", "-u", "vpn-master.service", "-u", "vpn-node.service", "-n", "50", "-f")
		return tea.ExecProcess(c, func(err error) tea.Msg {
			return extProcessFinishedMsg{action: "logs", err: err}
		})
	case 5:
		m.state = stateToolsMenu
		m.toolsChoice = 0
	case 6:
		// Запуск апдейтера
		c := exec.Command("bash", "-c", "if [ -f /tmp/aimatos-updater-bin ]; then /tmp/aimatos-updater-bin; else curl -sSL https://raw.githubusercontent.com/AimatosPanel/vpn-installer/main/update.sh | bash; fi")
		return tea.ExecProcess(c, func(err error) tea.Msg {
			return extProcessFinishedMsg{action: "update", err: err}
		})
	case 7:
		// Запуск деинсталлятора
		c := exec.Command("bash", "-c", "if [ -f /tmp/aimatos-uninstaller-bin ]; then /tmp/aimatos-uninstaller-bin; else curl -sSL https://raw.githubusercontent.com/AimatosPanel/vpn-installer/main/uninstall.sh | bash; fi")
		return tea.ExecProcess(c, func(err error) tea.Msg {
			return extProcessFinishedMsg{action: "uninstall", err: err}
		})
	}
	return nil
}

func (m *model) handleConfigMenu() {
	switch m.configChoice {
	case 0:
		m.currentSetting = "web_port"
		m.settingLabel = "Порт веб-интерфейса панели"
		m.input.SetValue(getMasterServicePort())
	case 1:
		m.currentSetting = "node_port"
		m.settingLabel = "Порт агента ноды (Node Agent)"
		m.input.SetValue(m.getSetting("node_port", "8085"))
	case 2:
		m.currentSetting = "reality_sni"
		m.settingLabel = "Reality SNI (Маскировочный домен)"
		m.input.SetValue(m.getSetting("reality_sni", "microsoft.com"))
	case 3:
		m.currentSetting = "vless_port"
		m.settingLabel = "Порт VLESS Reality (TCP)"
		m.input.SetValue(m.getSetting("vless_port", "8443"))
	case 4:
		m.currentSetting = "hysteria_port"
		m.settingLabel = "Порт Hysteria 2 (UDP)"
		m.input.SetValue(m.getSetting("hysteria_port", "8444"))
	case 5:
		m.currentSetting = "tuic_port"
		m.settingLabel = "Порт TUIC v5 (UDP)"
		m.input.SetValue(m.getSetting("tuic_port", "8445"))
	case 6:
		m.state = stateMain
		return
	}
	m.input.Focus()
	m.state = stateConfigEdit
}

func (m *model) saveConfigValue() {
	newVal := strings.TrimSpace(m.input.Value())
	if newVal == "" {
		m.outputMsg = "Ошибка: Значение не может быть пустым."
		m.state = stateConfigMenu
		return
	}

	if m.currentSetting == "web_port" {
		p, err := strconv.Atoi(newVal)
		if err != nil || p < 1 || p > 65535 {
			m.outputMsg = "Ошибка: Укажите корректный порт (1-65535)."
			m.state = stateConfigMenu
			return
		}
		_ = setMasterServicePort(newVal)
		_ = exec.Command("systemctl", "daemon-reload").Run()
		_ = exec.Command("systemctl", "restart", "vpn-master.service").Run()
		m.webPort = newVal
		m.outputMsg = fmt.Sprintf("Порт веб-панели изменен на %s! Служба перезапущена.", newVal)
	} else if strings.HasSuffix(m.currentSetting, "_port") {
		p, err := strconv.Atoi(newVal)
		if err != nil || p < 1 || p > 65535 {
			m.outputMsg = "Ошибка: Укажите корректный сетевой порт (1-65535)."
			m.state = stateConfigMenu
			return
		}
		_ = m.setSetting(m.currentSetting, newVal)
		_ = exec.Command("systemctl", "restart", "vpn-master.service", "vpn-node.service").Run()
		m.outputMsg = fmt.Sprintf("Параметр '%s' изменен на %s! Службы перезапущены.", m.settingLabel, newVal)
	} else {
		_ = m.setSetting(m.currentSetting, newVal)
		_ = exec.Command("systemctl", "restart", "vpn-master.service", "vpn-node.service").Run()
		m.outputMsg = fmt.Sprintf("Параметр '%s' обновлен! Конфигурация Sing-Box перезапущена.", m.settingLabel)
	}

	m.state = stateConfigMenu
}

func (m *model) handleToolsMenu() {
	switch m.toolsChoice {
	case 0:
		// Создание атомарного SQLite бэкапа
		_ = os.MkdirAll(BackupsDir, 0755)
		filename := filepath.Join(BackupsDir, fmt.Sprintf("panel_backup_%s.db", time.Now().Format("20060102_150405")))
		_, err := m.db.Exec(fmt.Sprintf("VACUUM INTO '%s';", filename))
		if err == nil {
			m.outputMsg = "Резервная копия БД создана: " + filepath.Base(filename)
		} else {
			m.outputMsg = "Ошибка создания бэкапа: " + err.Error()
		}
		m.state = stateMain

	case 1:
		m.reloadBackups()
		m.cursorIndex = 0
		m.state = stateBackupList

	case 2:
		// Активация BBR + FQ
		cmd := "echo 'net.core.default_qdisc=fq' >> /etc/sysctl.conf && echo 'net.ipv4.tcp_congestion_control=bbr' >> /etc/sysctl.conf && sysctl -p"
		_ = exec.Command("bash", "-c", cmd).Run()
		m.outputMsg = "Алгоритм TCP BBR + FQ успешно активирован в ядре!"
		m.state = stateMain

	case 3:
		_ = exec.Command("systemctl", "restart", "vpn-master.service", "vpn-node.service", "aimatos-port-hop.service").Run()
		m.outputMsg = "Все службы AimatosPanel успешно перезапущены!"
		m.state = stateMain

	case 4:
		m.state = stateMain
	}
}

func (m *model) restoreBackup(srcPath string) {
	_ = exec.Command("systemctl", "stop", "vpn-master.service", "vpn-node.service").Run()
	m.db.Close()

	cmd := fmt.Sprintf("cp --remove-destination %s %s", srcPath, DBPath)
	err := exec.Command("bash", "-c", cmd).Run()

	newDB, _ := sql.Open("sqlite", DBPath+"?_pragma=busy_timeout(5000)")
	m.db = newDB

	_ = exec.Command("systemctl", "restart", "vpn-master.service", "vpn-node.service").Run()

	if err == nil {
		m.outputMsg = "База данных успешно восстановлена из: " + filepath.Base(srcPath)
	} else {
		m.outputMsg = "Ошибка при восстановлении базы данных."
	}
	m.state = stateMain
}

// -------------------------------------------------------------
// РЕНДЕРИНГ ИНТЕРФЕЙСА (VIEW)
// -------------------------------------------------------------

func (m model) renderSysStats() string {
	up, _ := exec.Command("uptime", "-p").Output()
	mem, _ := exec.Command("bash", "-c", "free -h | awk '/^Mem:/ {print $3 \" / \" $2}'").Output()
	cpu, _ := exec.Command("bash", "-c", "top -bn1 | grep 'Cpu(s)' | awk '{print $2 + $4}'").Output()

	checkService := func(s string) string {
		out, _ := exec.Command("systemctl", "is-active", s).Output()
		if strings.TrimSpace(string(out)) == "active" {
			return successStyle.Render("● ACTIVE")
		}
		return failStyle.Render("○ INACTIVE")
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("  • Время работы (Uptime):  %s\n", strings.TrimSpace(string(up))))
	b.WriteString(fmt.Sprintf("  • Использование RAM:       %s\n", strings.TrimSpace(string(mem))))
	b.WriteString(fmt.Sprintf("  • Загрузка CPU:            %s%%\n\n", strings.TrimSpace(string(cpu))))
	b.WriteString(lipgloss.NewStyle().Bold(true).Render("  Состояние системных служб:") + "\n")
	b.WriteString(fmt.Sprintf("  ├─ Master Backend (API):  %s\n", checkService("vpn-master.service")))
	b.WriteString(fmt.Sprintf("  ├─ Node Agent (Sing-Box): %s\n", checkService("vpn-node.service")))
	b.WriteString(fmt.Sprintf("  └─ Port Hopping Rules:    %s\n", checkService("aimatos-port-hop.service")))
	return b.String()
}

func (m model) View() string {
	var s strings.Builder

	switch m.state {
	case stateMain:
		s.WriteString(titleStyle.Render("🔮  AIMATOS SYSTEM CONTROL CORE  🔮") + "\n")
		s.WriteString(subtitleStyle.Render("Серверный пульт администрирования сетевого узла") + "\n\n")

		if m.outputMsg != "" {
			s.WriteString(successStyle.Render("  [ ИНФО ]: "+m.outputMsg) + "\n\n")
		}

		options := []string{
			"Системный мониторинг и состояние служб",
			"Авторизация и вход в Веб-панель",
			"Параметры и ключи ноды (Подключение других панелей)",
			"Конфигурация портов и Reality SNI",
			"Журнал системных событий в реальном времени (Логи)",
			"Резервные копии и оптимизация ядра (BBR)",
			"Обновить AimatosPanel (Smart Auto-Updater)",
			"Полное удаление AimatosPanel с сервера",
		}

		for i, opt := range options {
			if i == m.mainChoice {
				s.WriteString(fmt.Sprintf("   %s  %s\n", focusStyle.Render("➔"), focusStyle.Render(fmt.Sprintf("[%d] %s", i+1, opt))))
			} else {
				s.WriteString(fmt.Sprintf("      %s\n", grayStyle.Render(fmt.Sprintf("[%d] %s", i+1, opt))))
			}
		}
		s.WriteString("\n" + helpStyle.Render(" [↑/↓] Навигация  •  [ ENTER ] Выбрать  •  [ Q ] Выход "))

	case stateStatus:
		s.WriteString(titleStyle.Render("🛰️  Мониторинг ресурсов и служб ") + "\n\n")
		s.WriteString(m.renderSysStats() + "\n")
		s.WriteString(helpStyle.Render(" Нажмите [ ENTER ] или [ ESC ] для возврата "))

	case stateLinks:
		s.WriteString(titleStyle.Render("🔗 Вход в панель управления ") + "\n\n")
		s.WriteString(fmt.Sprintf("  • Адрес веб-панели:  %s\n", successStyle.Render(fmt.Sprintf("http://%s:%s", m.serverIP, m.webPort))))
		s.WriteString(fmt.Sprintf("  • Секретный Ключ API: %s\n\n", focusStyle.Render(m.apiKey)))
		s.WriteString(grayStyle.Render("  Управление клиентами, создание ссылок и статистика\n  доступны через браузер по указанному выше адресу.") + "\n\n")
		s.WriteString(helpStyle.Render(" Нажмите [ ENTER ] или [ ESC ] для возврата "))

	case stateNodeCluster:
		s.WriteString(titleStyle.Render("🌐 Параметры ноды для подключения внешних панелей ") + "\n\n")
		s.WriteString("  Используйте эти реквизиты, если хотите подключить текущий сервер\n  как независимый узел к внешней мастер-панели управления:\n\n")
		s.WriteString(fmt.Sprintf("  • Внешний IP ноды:     %s\n", successStyle.Render(m.serverIP)))
		s.WriteString(fmt.Sprintf("  • Порт агента (Agent): %s\n", focusStyle.Render(m.nodePort)))
		s.WriteString(fmt.Sprintf("  • Ключ доступа API:    %s\n", focusStyle.Render(m.apiKey)))
		s.WriteString(fmt.Sprintf("  • URL телеметрии:      %s\n\n", grayStyle.Render(fmt.Sprintf("http://%s:%s/api/node/status", m.serverIP, m.nodePort))))
		s.WriteString(helpStyle.Render(" Нажмите [ ENTER ] или [ ESC ] для возврата "))

	case stateConfigMenu:
		s.WriteString(titleStyle.Render("⚙️ Конфигурация портов и сетевого ядра ") + "\n\n")
		if m.outputMsg != "" {
			s.WriteString(successStyle.Render("  [ ИНФО ]: "+m.outputMsg) + "\n\n")
		}

		items := []struct{ label, val string }{
			{"Порт веб-интерфейса", m.webPort},
			{"Порт агента ноды (Node)", m.getSetting("node_port", "8085")},
			{"Reality SNI (Маскировка)", m.getSetting("reality_sni", "microsoft.com")},
			{"Порт VLESS Reality (TCP)", m.getSetting("vless_port", "8443")},
			{"Порт Hysteria 2 (UDP)", m.getSetting("hysteria_port", "8444")},
			{"Порт TUIC v5 (UDP)", m.getSetting("tuic_port", "8445")},
		}

		for i, item := range items {
			line := fmt.Sprintf("%-28s : %s", item.label, successStyle.Render(item.val))
			if i == m.configChoice {
				s.WriteString(fmt.Sprintf(" ➔ %s\n", focusStyle.Render(line)))
			} else {
				s.WriteString(fmt.Sprintf("    %s\n", line))
			}
		}

		if m.configChoice == 6 {
			s.WriteString(fmt.Sprintf("\n ➔ %s\n", focusStyle.Render("Назад в главное меню")))
		} else {
			s.WriteString(fmt.Sprintf("\n    %s\n", "Назад в главное меню"))
		}
		s.WriteString("\n" + helpStyle.Render(" [ ENTER ] Изменить значение  •  [ ESC ] Назад "))

	case stateConfigEdit:
		s.WriteString(titleStyle.Render("✏️ Изменение системного параметра ") + "\n\n")
		s.WriteString(fmt.Sprintf("  Параметр: %s\n", focusStyle.Render(m.settingLabel)))
		s.WriteString(fmt.Sprintf("  Значение: %s\n\n", m.input.View()))
		s.WriteString(helpStyle.Render(" [ ENTER ] Применить и перезапустить службы  •  [ ESC ] Отмена "))

	case stateToolsMenu:
		s.WriteString(titleStyle.Render("🛠️ Системные инструменты и бэкапы ") + "\n\n")
		options := []string{
			"Создать резервную копию базы данных (Snapshot)",
			"Восстановить базу данных из сохранённых копий",
			"Активировать алгоритм TCP BBR + FQ в ядре Linux",
			"Перезапустить все службы AimatosPanel",
			"Назад в главное меню",
		}
		for i, opt := range options {
			if i == m.toolsChoice {
				s.WriteString(fmt.Sprintf("   %s  %s\n", focusStyle.Render("➔"), focusStyle.Render(opt)))
			} else {
				s.WriteString(fmt.Sprintf("      %s\n", opt))
			}
		}
		s.WriteString("\n" + helpStyle.Render(" [↑/↓] Навигация  •  [ ENTER ] Выполнить  •  [ ESC ] Назад "))

	case stateBackupList:
		s.WriteString(titleStyle.Render("💾 Восстановление резервной копии базы данных ") + "\n\n")
		if len(m.backups) == 0 {
			s.WriteString("  В папке /opt/aimatos/backups/ нет доступных файлов копий.\n\n")
		} else {
			for i, b := range m.backups {
				line := fmt.Sprintf("%-28s | %s | %s", b.Name, b.Size, b.Time)
				if i == m.cursorIndex {
					s.WriteString(fmt.Sprintf(" ➔ %s\n", focusStyle.Render(line)))
				} else {
					s.WriteString(fmt.Sprintf("    %s\n", line))
				}
			}
		}
		s.WriteString("\n" + helpStyle.Render(" [ ENTER ] Восстановить выбранный бэкап  •  [ ESC ] Назад "))
	}

	innerBox := windowStyle.Render(s.String())
	return lipgloss.Place(m.termWidth, m.termHeight, lipgloss.Center, lipgloss.Center, innerBox)
}

func main() {
	if os.Geteuid() != 0 {
		fmt.Println("❌ Ошибка: Запуск утилиты aimatos требует прав root.")
		os.Exit(1)
	}

	p := tea.NewProgram(initialModel())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Критический сбой TUI: %v\n", err)
		os.Exit(1)
	}
}