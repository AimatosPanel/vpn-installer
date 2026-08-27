package main

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
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
	DBPath     = "/opt/aimatos/vpn-master/panel.db"
	BackupsDir = "/opt/aimatos/backups"
)

// Цветовая палитра Aimatos Cyberpunk
var (
	accentColor  = lipgloss.Color("99")  // Фиолетовый
	pinkColor    = lipgloss.Color("205") // Розовый
	grayColor    = lipgloss.Color("244")
	successColor = lipgloss.Color("46")  // Зеленый
	failColor    = lipgloss.Color("196") // Красный
	amberColor   = lipgloss.Color("214") // Оранжевый

	titleStyle    = lipgloss.NewStyle().Foreground(pinkColor).Bold(true).Align(lipgloss.Center)
	subtitleStyle = lipgloss.NewStyle().Foreground(grayColor).Align(lipgloss.Center)
	windowStyle   = lipgloss.NewStyle().Border(lipgloss.DoubleBorder()).BorderForeground(accentColor).Padding(1, 3).Width(72)
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
	stateUsersMenu
	stateUserList
	stateUserDetail
	stateUserAdd
	stateUserToggle
	stateUserReset
	stateUserDelete
	statePortsMenu
	statePortEdit
	stateToolsMenu
	stateBackupList
)

type UserItem struct {
	ID        int64
	Name      string
	IsActive  bool
	UUID      string
	Pass      string
	LimitGB   float64
	UsedBytes int64
	ExpiresAt string
}

type BackupItem struct {
	Name string
	Path string
	Size string
	Time string
}

type logFinishedMsg struct{ err error }

type model struct {
	state          menuState
	mainChoice     int
	userChoice     int
	toolsChoice    int
	portsChoice    int
	cursorIndex    int
	inputs         []textinput.Model
	activeInput    int
	spinner        spinner.Model
	db             *sql.DB
	termWidth      int
	termHeight     int
	outputMsg      string
	apiKey         string
	serverIP       string
	users          []UserItem
	selectedUser   *UserItem
	backups        []BackupItem
	currentSetting string
}

// -------------------------------------------------------------
// ВСПОМОГАТЕЛЬНЫЕ ФУНКЦИИ (ГЕНЕРАТОРЫ И УТИЛИТЫ)
// -------------------------------------------------------------

func generateUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // RFC 4122 v4
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func generatePassword(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = letters[int(b[i])%len(letters)]
	}
	return string(b)
}

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

func (m *model) reloadUsers() {
	m.users = nil
	if m.db == nil {
		return
	}
	rows, err := m.db.Query("SELECT id, name, is_active, vless_uuid, hysteria2_password, traffic_limit_gb, traffic_used_bytes, COALESCE(expires_at, 'Бессрочно') FROM users ORDER BY id DESC")
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var u UserItem
		var activeInt int
		if err := rows.Scan(&u.ID, &u.Name, &activeInt, &u.UUID, &u.Pass, &u.LimitGB, &u.UsedBytes, &u.ExpiresAt); err == nil {
			u.IsActive = activeInt == 1
			m.users = append(m.users, u)
		}
	}
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
// ИНИЦИАЛИЗАЦИЯ И МЕНЮ
// -------------------------------------------------------------

func initialModel() model {
	db, err := sql.Open("sqlite", DBPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		fmt.Printf("Ошибка подключения к БД: %v\n", err)
		os.Exit(1)
	}

	inputs := make([]textinput.Model, 4)
	// Создание пользователя
	inputs[0] = textinput.New()
	inputs[0].Placeholder = "имя_клиента (латиница)"
	inputs[0].CharLimit = 20
	inputs[0].Width = 22

	inputs[1] = textinput.New()
	inputs[1].Placeholder = "0 — безлимит"
	inputs[1].CharLimit = 6
	inputs[1].Width = 15

	inputs[2] = textinput.New()
	inputs[2].Placeholder = "0 — бессрочно"
	inputs[2].CharLimit = 6
	inputs[2].Width = 15

	// Редактирование порта
	inputs[3] = textinput.New()
	inputs[3].Placeholder = "Порт (1-65535)"
	inputs[3].CharLimit = 5
	inputs[3].Width = 15

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(accentColor)

	m := model{
		state:       stateMain,
		mainChoice:  0,
		inputs:      inputs,
		spinner:     s,
		db:          db,
		serverIP:    getPublicIP(),
		cursorIndex: 0,
	}

	m.apiKey = m.getSetting("api_key", "SuperSecretAdminKey123")
	m.reloadUsers()
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

	case logFinishedMsg:
		m.state = stateMain
		if msg.err != nil {
			m.outputMsg = "Журнал закрыт: " + msg.err.Error()
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
				if m.mainChoice < 8 {
					m.mainChoice++
				}
			case "enter":
				cmd := m.handleMainMenu()
				if cmd != nil {
					return m, cmd
				}
			}

		case stateUsersMenu:
			switch msg.String() {
			case "up", "k":
				if m.userChoice > 0 {
					m.userChoice--
				}
			case "down", "j":
				if m.userChoice < 5 {
					m.userChoice++
				}
			case "enter":
				m.handleUsersMenu()
			}

		case stateUserList:
			switch msg.String() {
			case "up", "k":
				if m.cursorIndex > 0 {
					m.cursorIndex--
				}
			case "down", "j":
				if m.cursorIndex < len(m.users)-1 {
					m.cursorIndex++
				}
			case "enter":
				if len(m.users) > 0 && m.cursorIndex < len(m.users) {
					u := m.users[m.cursorIndex]
					m.selectedUser = &u
					m.state = stateUserDetail
				}
			case "esc":
				m.state = stateUsersMenu
			}

		case stateUserDetail:
			if msg.String() == "enter" || msg.String() == "esc" {
				m.state = stateUserList
			}

		case stateUserAdd:
			switch msg.String() {
			case "tab", "shift+tab":
				m.inputs[m.activeInput].Blur()
				m.activeInput = (m.activeInput + 1) % 3
				m.inputs[m.activeInput].Focus()
			case "enter":
				m.createNewUser()
			case "esc":
				m.state = stateUsersMenu
			}
			var cmd tea.Cmd
			m.inputs[m.activeInput], cmd = m.inputs[m.activeInput].Update(msg)
			return m, cmd

		case stateUserToggle:
			switch msg.String() {
			case "up", "k":
				if m.cursorIndex > 0 {
					m.cursorIndex--
				}
			case "down", "j":
				if m.cursorIndex < len(m.users)-1 {
					m.cursorIndex++
				}
			case "enter":
				if len(m.users) > 0 {
					target := m.users[m.cursorIndex]
					newAct := 0
					if !target.IsActive {
						newAct = 1
					}
					_, _ = m.db.Exec("UPDATE users SET is_active = ? WHERE id = ?", newAct, target.ID)
					m.reloadUsers()
					m.outputMsg = fmt.Sprintf("Статус клиента '%s' изменен!", target.Name)
					m.state = stateUsersMenu
				}
			case "esc":
				m.state = stateUsersMenu
			}

		case stateUserReset:
			switch msg.String() {
			case "up", "k":
				if m.cursorIndex > 0 {
					m.cursorIndex--
				}
			case "down", "j":
				if m.cursorIndex < len(m.users) { // +1 для сброса всех
					m.cursorIndex++
				}
			case "enter":
				if m.cursorIndex == len(m.users) {
					_, _ = m.db.Exec("UPDATE users SET traffic_used_bytes = 0, traffic_uplink_bytes = 0, traffic_downlink_bytes = 0")
					m.outputMsg = "Трафик ВСЕХ клиентов успешно сброшен на ноль!"
				} else if len(m.users) > 0 {
					target := m.users[m.cursorIndex]
					_, _ = m.db.Exec("UPDATE users SET traffic_used_bytes = 0, traffic_uplink_bytes = 0, traffic_downlink_bytes = 0 WHERE id = ?", target.ID)
					m.outputMsg = fmt.Sprintf("Трафик клиента '%s' сброшен на 0!", target.Name)
				}
				m.reloadUsers()
				m.state = stateUsersMenu
			case "esc":
				m.state = stateUsersMenu
			}

		case stateUserDelete:
			switch msg.String() {
			case "up", "k":
				if m.cursorIndex > 0 {
					m.cursorIndex--
				}
			case "down", "j":
				if m.cursorIndex < len(m.users)-1 {
					m.cursorIndex++
				}
			case "enter":
				if len(m.users) > 0 {
					target := m.users[m.cursorIndex]
					_, _ = m.db.Exec("DELETE FROM users WHERE id = ?", target.ID)
					m.reloadUsers()
					m.outputMsg = fmt.Sprintf("Клиент '%s' полностью удален!", target.Name)
					m.state = stateUsersMenu
				}
			case "esc":
				m.state = stateUsersMenu
			}

		case statePortsMenu:
			switch msg.String() {
			case "up", "k":
				if m.portsChoice > 0 {
					m.portsChoice--
				}
			case "down", "j":
				if m.portsChoice < 5 {
					m.portsChoice++
				}
			case "enter":
				m.handlePortsMenu()
			case "esc":
				m.state = stateMain
			}

		case statePortEdit:
			switch msg.String() {
			case "enter":
				m.saveNewPort()
			case "esc":
				m.state = statePortsMenu
			}
			var cmd tea.Cmd
			m.inputs[3], cmd = m.inputs[3].Update(msg)
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
	case 2:
		m.state = stateUsersMenu
		m.userChoice = 0
		m.reloadUsers()
	case 3:
		m.state = statePortsMenu
		m.portsChoice = 0
	case 4:
		// Реальный стриминг системного журнала
		c := exec.Command("journalctl", "-u", "vpn-master.service", "-u", "vpn-node.service", "-n", "50", "-f")
		return tea.ExecProcess(c, func(err error) tea.Msg {
			return logFinishedMsg{err}
		})
	case 5:
		m.state = stateToolsMenu
		m.toolsChoice = 0
	case 6:
		// Запуск умного апдейтера
		c := exec.Command("bash", "-c", "if [ -f /tmp/aimatos-updater-bin ]; then /tmp/aimatos-updater-bin; else curl -sSL https://raw.githubusercontent.com/AimatosPanel/vpn-installer/main/update.sh | bash; fi")
		return tea.ExecProcess(c, func(err error) tea.Msg {
			return logFinishedMsg{err}
		})
	case 7:
		// Запуск деинсталлятора
		c := exec.Command("bash", "-c", "if [ -f /tmp/aimatos-uninstaller-bin ]; then /tmp/aimatos-uninstaller-bin; else curl -sSL https://raw.githubusercontent.com/AimatosPanel/vpn-installer/main/uninstall.sh | bash; fi")
		return tea.ExecProcess(c, func(err error) tea.Msg {
			return logFinishedMsg{err}
		})
	case 8:
		if m.db != nil {
			m.db.Close()
		}
		os.Exit(0)
	}
	return nil
}

func (m *model) handleUsersMenu() {
	m.cursorIndex = 0
	m.reloadUsers()
	switch m.userChoice {
	case 0:
		m.state = stateUserList
	case 1:
		m.state = stateUserAdd
		m.inputs[0].SetValue("")
		m.inputs[1].SetValue("")
		m.inputs[2].SetValue("")
		m.inputs[0].Focus()
		m.activeInput = 0
	case 2:
		m.state = stateUserToggle
	case 3:
		m.state = stateUserReset
	case 4:
		m.state = stateUserDelete
	case 5:
		m.state = stateMain
	}
}

func (m *model) handlePortsMenu() {
	if m.portsChoice == 5 {
		m.state = stateMain
		return
	}
	keys := []string{"vless_port", "vless_grpc_port", "hysteria_port", "tuic_port", "naive_port"}
	m.currentSetting = keys[m.portsChoice]
	m.inputs[3].SetValue(m.getSetting(m.currentSetting, "8443"))
	m.inputs[3].Focus()
	m.state = statePortEdit
}

func (m *model) saveNewPort() {
	valStr := strings.TrimSpace(m.inputs[3].Value())
	p, err := strconv.Atoi(valStr)
	if err != nil || p < 1 || p > 65535 {
		m.outputMsg = "Ошибка: Укажите корректный порт от 1 до 65535."
		m.state = statePortsMenu
		return
	}

	_ = m.setSetting(m.currentSetting, valStr)
	_ = exec.Command("systemctl", "restart", "vpn-master.service", "vpn-node.service").Run()
	m.outputMsg = fmt.Sprintf("Порт для '%s' изменен на %d! Службы перезапущены.", m.currentSetting, p)
	m.state = statePortsMenu
}

func (m *model) handleToolsMenu() {
	switch m.toolsChoice {
	case 0:
		// Бэкап SQLite (VACUUM INTO)
		_ = os.MkdirAll(BackupsDir, 0755)
		filename := filepath.Join(BackupsDir, fmt.Sprintf("panel_backup_%s.db", time.Now().Format("20060102_150405")))
		_, err := m.db.Exec(fmt.Sprintf("VACUUM INTO '%s';", filename))
		if err == nil {
			m.outputMsg = "Резервная копия успешно создана в: " + filename
		} else {
			m.outputMsg = "Ошибка создания бэкапа: " + err.Error()
		}
		m.state = stateMain

	case 1:
		m.reloadBackups()
		m.cursorIndex = 0
		m.state = stateBackupList

	case 2:
		// BBR + FQ
		cmd := "echo 'net.core.default_qdisc=fq' >> /etc/sysctl.conf && echo 'net.ipv4.tcp_congestion_control=bbr' >> /etc/sysctl.conf && sysctl -p"
		_ = exec.Command("bash", "-c", cmd).Run()
		m.outputMsg = "Сетевой алгоритм TCP BBR + FQ успешно активирован в ядре!"
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
	m.reloadUsers()

	if err == nil {
		m.outputMsg = "База данных успешно восстановлена из: " + filepath.Base(srcPath)
	} else {
		m.outputMsg = "Ошибка при восстановлении базы данных."
	}
	m.state = stateMain
}

func (m *model) createNewUser() {
	name := strings.TrimSpace(strings.ToLower(m.inputs[0].Value()))
	if name == "" {
		m.outputMsg = "Ошибка: Имя клиента не может быть пустым."
		m.state = stateUsersMenu
		return
	}

	limitGB, _ := strconv.ParseFloat(strings.TrimSpace(m.inputs[1].Value()), 64)
	days, _ := strconv.Atoi(strings.TrimSpace(m.inputs[2].Value()))

	var expStr *string
	if days > 0 {
		formatted := time.Now().AddDate(0, 0, days).Format("2006-01-02 15:04:05")
		expStr = &formatted
	}

	uuidStr := generateUUID()
	passStr := generatePassword(16)

	_, err := m.db.Exec(`INSERT INTO users (name, is_active, vless_uuid, hysteria2_password, traffic_limit_gb, expires_at, allowed_protocols) 
		VALUES (?, 1, ?, ?, ?, ?, 'vless,hysteria2,tuic,naive')`, name, uuidStr, passStr, limitGB, expStr)

	if err == nil {
		m.outputMsg = fmt.Sprintf("Клиент '%s' успешно сгенерирован и активен!", name)
	} else {
		m.outputMsg = "Ошибка создания: клиент с таким именем уже существует."
	}

	m.reloadUsers()
	m.state = stateUsersMenu
}

// -------------------------------------------------------------
// РЕНДЕРИНГ ИНТЕРФЕЙСА
// -------------------------------------------------------------

func (m model) renderSysStats() string {
	up, _ := exec.Command("uptime", "-p").Output()
	mem, _ := exec.Command("bash", "-c", "free -h | awk '/^Mem:/ {print $3 \" / \" $2}'").Output()
	cpu, _ := exec.Command("bash", "-c", "top -bn1 | grep 'Cpu(s)' | awk '{print $2 + $4}'").Output()

	// Проверка активности демонов
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

func (m model) renderUserLinks(u *UserItem) string {
	vlessPort := m.getSetting("vless_port", "8443")
	hy2Port := m.getSetting("hysteria_port", "8444")
	tuicPort := m.getSetting("tuic_port", "8445")
	realitySNI := m.getSetting("reality_sni", "microsoft.com")
	realityPubKey := m.getSetting("reality_public_key", "")
	realityShortID := m.getSetting("reality_short_id", "")
	hy2Obfs := m.getSetting("hysteria_obfs", "ObfsSecretPass123")

	vlessLink := fmt.Sprintf("vless://%s@%s:%s?security=reality&encryption=none&pbk=%s&headerType=none&fp=chrome&spx=%%2F&type=tcp&sni=%s&sid=%s&flow=xtls-rprx-vision#Reality-%s",
		u.UUID, m.serverIP, vlessPort, realityPubKey, realitySNI, realityShortID, u.Name)

	hy2Link := fmt.Sprintf("hysteria2://%s@%s:%s?insecure=1&sni=%s&obfs=salamander&obfs-password=%s#Hysteria2-%s",
		u.Pass, m.serverIP, hy2Port, realitySNI, hy2Obfs, u.Name)

	tuicLink := fmt.Sprintf("tuic://%s:%s@%s:%s?congestion_control=bbr&alpn=h3&sni=%s&allow_insecure=1#TUIC-%s",
		u.UUID, u.Pass, m.serverIP, tuicPort, realitySNI, u.Name)

	subURL := fmt.Sprintf("http://%s:8080/sub/%s", m.serverIP, u.UUID)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("  Клиент: %s\n", focusStyle.Render(u.Name)))
	b.WriteString(fmt.Sprintf("  Статус: %s  •  Лимит: %.2f GB  •  Срок: %s\n\n",
		map[bool]string{true: successStyle.Render("АКТИВЕН"), false: failStyle.Render("ОТКЛЮЧЕН")}[u.IsActive], u.LimitGB, u.ExpiresAt))

	b.WriteString(lipgloss.NewStyle().Bold(true).Render("  🔗 Универсальная ссылка подписки:") + "\n")
	b.WriteString(fmt.Sprintf("  %s\n\n", successStyle.Render(subURL)))

	b.WriteString(lipgloss.NewStyle().Bold(true).Render("  🔑 Прямые ключи подключения:") + "\n")
	b.WriteString(fmt.Sprintf("  • VLESS Reality: %s\n\n", grayStyle.Render(vlessLink)))
	b.WriteString(fmt.Sprintf("  • Hysteria 2:    %s\n\n", grayStyle.Render(hy2Link)))
	b.WriteString(fmt.Sprintf("  • TUIC v5:       %s\n\n", grayStyle.Render(tuicLink)))
	return b.String()
}

func (m model) View() string {
	var s strings.Builder

	switch m.state {
	case stateMain:
		s.WriteString(titleStyle.Render("🔮  AIMATOS MASTER CONTROL CLI  🔮") + "\n")
		s.WriteString(subtitleStyle.Render("Единый пульт администрирования сетевого ядра") + "\n\n")

		if m.outputMsg != "" {
			s.WriteString(successStyle.Render("  [ ИНФО ]: "+m.outputMsg) + "\n\n")
		}

		options := []string{
			"Системный мониторинг и состояние служб",
			"Параметры веб-панели и Ключ API",
			"Управление клиентами VPN (Создание / Ключи)",
			"Переназначение сетевых портов",
			"Журнал системных событий в реальном времени (Логи)",
			"Резервные копии и оптимизация ядра (BBR)",
			"Обновить AimatosPanel до последней версии",
			"Полное удаление AimatosPanel с сервера",
			"Выйти из утилиты управления",
		}

		for i, opt := range options {
			if i == m.mainChoice {
				s.WriteString(fmt.Sprintf("   %s  %s\n", focusStyle.Render("➔"), focusStyle.Render(fmt.Sprintf("[%d] %s", i+1, opt))))
			} else {
				s.WriteString(fmt.Sprintf("      %s\n", grayStyle.Render(fmt.Sprintf("[%d] %s", i+1, opt))))
			}
		}
		s.WriteString("\n" + helpStyle.Render(" [↑/↓] Навигация  •  [ ENTER ] Подтвердить  •  [ Q ] Выход "))

	case stateStatus:
		s.WriteString(titleStyle.Render("🛰️  Мониторинг ресурсов и служб ") + "\n\n")
		s.WriteString(m.renderSysStats() + "\n")
		s.WriteString(helpStyle.Render(" Нажмите [ ENTER ] для возврата в меню "))

	case stateLinks:
		s.WriteString(titleStyle.Render("🔗 Авторизация и доступ к панели ") + "\n\n")
		s.WriteString(fmt.Sprintf("  • Адрес веб-интерфейса: %s\n", successStyle.Render(fmt.Sprintf("http://%s:8080", m.serverIP))))
		s.WriteString(fmt.Sprintf("  • Секретный Ключ API:   %s\n\n", focusStyle.Render(m.apiKey)))
		s.WriteString(helpStyle.Render(" Нажмите [ ENTER ] для возврата в меню "))

	case stateUsersMenu:
		s.WriteString(titleStyle.Render("👥 Управление базой клиентов ") + "\n\n")
		if m.outputMsg != "" {
			s.WriteString(successStyle.Render("  [ ИНФО ]: "+m.outputMsg) + "\n\n")
		}
		options := []string{
			fmt.Sprintf("Список клиентов и получение ключей (%d)", len(m.users)),
			"Сгенерировать нового клиента (VLESS, Hy2, TUIC)",
			"Включить / Отключить клиента",
			"Сбросить израсходованный трафик",
			"Удалить клиента из базы",
			"Назад в главное меню",
		}
		for i, opt := range options {
			if i == m.userChoice {
				s.WriteString(fmt.Sprintf("   %s  %s\n", focusStyle.Render("➔"), focusStyle.Render(opt)))
			} else {
				s.WriteString(fmt.Sprintf("      %s\n", opt))
			}
		}
		s.WriteString("\n" + helpStyle.Render(" [↑/↓] Навигация  •  [ ENTER ] Выбрать "))

	case stateUserList:
		s.WriteString(titleStyle.Render("👥 База клиентов (Выберите для просмотра ключей) ") + "\n\n")
		if len(m.users) == 0 {
			s.WriteString("  Список клиентов пуст.\n\n")
		} else {
			for i, u := range m.users {
				status := "🟢"
				if !u.IsActive {
					status = "🔴"
				}
				limStr := "Безлимит"
				if u.LimitGB > 0 {
					limStr = fmt.Sprintf("%.1f GB", u.LimitGB)
				}
				line := fmt.Sprintf("%s %-16s | %s / %s | Срок: %s", status, u.Name, formatBytes(u.UsedBytes), limStr, u.ExpiresAt)
				if i == m.cursorIndex {
					s.WriteString(fmt.Sprintf(" ➔ %s\n", focusStyle.Render(line)))
				} else {
					s.WriteString(fmt.Sprintf("    %s\n", grayStyle.Render(line)))
				}
			}
		}
		s.WriteString("\n" + helpStyle.Render(" [↑/↓] Выбор  •  [ ENTER ] Ключи и ссылки  •  [ ESC ] Назад "))

	case stateUserDetail:
		s.WriteString(titleStyle.Render("🔑 Конфигурация подключения клиента ") + "\n\n")
		if m.selectedUser != nil {
			s.WriteString(m.renderUserLinks(m.selectedUser))
		}
		s.WriteString(helpStyle.Render(" Нажмите [ ENTER ] для возврата к списку "))

	case stateUserAdd:
		s.WriteString(titleStyle.Render("👤 Генерация нового VPN-клиента ") + "\n\n")
		s.WriteString(fmt.Sprintf("  • Имя пользователя : %s\n", m.inputs[0].View()))
		s.WriteString(fmt.Sprintf("  • Лимит трафика ГБ : %s\n", m.inputs[1].View()))
		s.WriteString(fmt.Sprintf("  • Срок работы (дни): %s\n\n", m.inputs[2].View()))
		s.WriteString(helpStyle.Render(" [ TAB ] Сменить поле  •  [ ENTER ] Создать  •  [ ESC ] Отмена "))

	case stateUserToggle:
		s.WriteString(titleStyle.Render("⚡ Переключение активности клиента ") + "\n\n")
		for i, u := range m.users {
			status := map[bool]string{true: successStyle.Render("АКТИВЕН"), false: failStyle.Render("ОТКЛЮЧЕН")}[u.IsActive]
			line := fmt.Sprintf("%-16s [ %s ]", u.Name, status)
			if i == m.cursorIndex {
				s.WriteString(fmt.Sprintf(" ➔ %s\n", focusStyle.Render(line)))
			} else {
				s.WriteString(fmt.Sprintf("    %s\n", line))
			}
		}
		s.WriteString("\n" + helpStyle.Render(" [ ENTER ] Переключить статус  •  [ ESC ] Назад "))

	case stateUserReset:
		s.WriteString(titleStyle.Render("🔄 Сброс израсходованного трафика ") + "\n\n")
		for i, u := range m.users {
			line := fmt.Sprintf("%-16s | Использовано: %s", u.Name, formatBytes(u.UsedBytes))
			if i == m.cursorIndex {
				s.WriteString(fmt.Sprintf(" ➔ %s\n", focusStyle.Render(line)))
			} else {
				s.WriteString(fmt.Sprintf("    %s\n", line))
			}
		}
		resetAllText := "⚠️  СБРОСИТЬ ТРАФИК ВСЕХ КЛИЕНТОВ НА 0"
		if m.cursorIndex == len(m.users) {
			s.WriteString(fmt.Sprintf("\n ➔ %s\n", failStyle.Render(resetAllText)))
		} else {
			s.WriteString(fmt.Sprintf("\n    %s\n", amberColor))
		}
		s.WriteString("\n" + helpStyle.Render(" [ ENTER ] Сбросить счетчик  •  [ ESC ] Назад "))

	case stateUserDelete:
		s.WriteString(titleStyle.Render("🗑️  Удаление профиля клиента ") + "\n\n")
		for i, u := range m.users {
			line := fmt.Sprintf("%-16s (ID: %d)", u.Name, u.ID)
			if i == m.cursorIndex {
				s.WriteString(fmt.Sprintf(" ➔ %s\n", failStyle.Render(line)))
			} else {
				s.WriteString(fmt.Sprintf("    %s\n", line))
			}
		}
		s.WriteString("\n" + helpStyle.Render(" [ ENTER ] Удалить навсегда  •  [ ESC ] Назад "))

	case statePortsMenu:
		s.WriteString(titleStyle.Render("⚙️ Сетевые порты сетевых служб ") + "\n\n")
		ports := []struct{ name, key, def string }{
			{"VLESS Reality (TCP)", "vless_port", "8443"},
			{"VLESS Reality (gRPC)", "vless_grpc_port", "8447"},
			{"Hysteria 2 (UDP)", "hysteria_port", "8444"},
			{"TUIC v5 (UDP)", "tuic_port", "8445"},
			{"NaiveProxy (TCP)", "naive_port", "8446"},
		}
		for i, p := range ports {
			val := m.getSetting(p.key, p.def)
			line := fmt.Sprintf("%-24s : %s", p.name, successStyle.Render(val))
			if i == m.portsChoice {
				s.WriteString(fmt.Sprintf(" ➔ %s\n", focusStyle.Render(line)))
			} else {
				s.WriteString(fmt.Sprintf("    %s\n", line))
			}
		}
		if m.portsChoice == 5 {
			s.WriteString(fmt.Sprintf("\n ➔ %s\n", focusStyle.Render("Назад в главное меню")))
		} else {
			s.WriteString(fmt.Sprintf("\n    %s\n", "Назад в главное меню"))
		}
		s.WriteString("\n" + helpStyle.Render(" [ ENTER ] Изменить порт  •  [ ESC ] Назад "))

	case statePortEdit:
		s.WriteString(titleStyle.Render("✏️ Изменение сетевого порта ") + "\n\n")
		s.WriteString(fmt.Sprintf("  Параметр: %s\n", focusStyle.Render(m.currentSetting)))
		s.WriteString(fmt.Sprintf("  Новый порт: %s\n\n", m.inputs[3].View()))
		s.WriteString(helpStyle.Render(" [ ENTER ] Сохранить и перезапустить службы  •  [ ESC ] Отмена "))

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
		s.WriteString("\n" + helpStyle.Render(" [↑/↓] Навигация  •  [ ENTER ] Выполнить "))

	case stateBackupList:
		s.WriteString(titleStyle.Render("💾 Выберите резервную копию для восстановления ") + "\n\n")
		if len(m.backups) == 0 {
			s.WriteString("  В папке /opt/aimatos/backups/ нет доступных копий.\n\n")
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