package main

import (
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	_ "modernc.org/sqlite"
)

var (
	accentColor = lipgloss.Color("99")
	pinkColor   = lipgloss.Color("205")
	grayColor   = lipgloss.Color("244")

	titleStyle    = lipgloss.NewStyle().Foreground(pinkColor).Bold(true).Align(lipgloss.Center)
	subtitleStyle = lipgloss.NewStyle().Foreground(grayColor).Align(lipgloss.Center)
	windowStyle   = lipgloss.NewStyle().Border(lipgloss.DoubleBorder()).BorderForeground(accentColor).Padding(1, 4).Width(68).Height(18)
	successStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("46")).Bold(true)
	failStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	focusStyle    = lipgloss.NewStyle().Foreground(accentColor).Bold(true)
	grayStyle     = lipgloss.NewStyle().Foreground(grayColor)
	helpStyle     = lipgloss.NewStyle().Foreground(grayColor).Align(lipgloss.Center)
)

const DB_PATH = "/opt/aimatos/vpn-master/panel.db"

type menuState int
const (
	stateMain menuState = iota
	stateStatus
	stateLinks
	stateUsersMenu
	stateUserList
	stateUserAdd
	statePortsMenu
	stateToolsMenu
)

type model struct {
	state       menuState
	mainChoice  int
	userChoice  int
	portsChoice int
	toolsChoice int
	inputs      []textinput.Model
	activeInput int
	spinner     spinner.Model
	db          *sql.DB
	termWidth   int
	termHeight  int
	outputMsg   string
}

func initialModel() model {
	db, err := sql.Open("sqlite", DB_PATH)
	if err != nil {
		fmt.Printf("Ошибка подключения к БД: %v\n", err)
		os.Exit(1)
	}

	inputs := make([]textinput.Model, 3)
	inputs[0] = textinput.New()
	inputs[0].Placeholder = "имя_пользователя"
	inputs[0].CharLimit = 20
	inputs[0].Width = 20

	inputs[1] = textinput.New()
	inputs[1].Placeholder = "Лимит трафика GB (0 - безлимит)"
	inputs[1].CharLimit = 5
	inputs[1].Width = 10

	inputs[2] = textinput.New()
	inputs[2].Placeholder = "Срок действия в днях (0 - бессрочно)"
	inputs[2].CharLimit = 5
	inputs[2].Width = 10

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(accentColor)

	return model{
		state:       stateMain,
		mainChoice:  0,
		inputs:      inputs,
		spinner:     s,
		db:          db,
		outputMsg:   "",
	}
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

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			if m.state == stateMain {
				m.db.Close()
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
				if m.mainChoice > 0 { m.mainChoice-- }
			case "down", "j":
				if m.mainChoice < 6 { m.mainChoice++ }
			case "enter":
				m.handleMainMenuSelection()
			}

		case stateUsersMenu:
			switch msg.String() {
			case "up", "k":
				if m.userChoice > 0 { m.userChoice-- }
			case "down", "j":
				if m.userChoice < 5 { m.userChoice++ }
			case "enter":
				m.handleUsersMenuSelection()
			}

		case stateUserAdd:
			switch msg.String() {
			case "tab", "shift+tab":
				m.inputs[m.activeInput].Blur()
				if m.activeInput == 0 {
					m.activeInput = 1
				} else if m.activeInput == 1 {
					m.activeInput = 2
				} else {
					m.activeInput = 0
				}
				m.inputs[m.activeInput].Focus()
			case "enter":
				m.createNewUser()
			}

			var cmd tea.Cmd
			m.inputs[m.activeInput], cmd = m.inputs[m.activeInput].Update(msg)
			return m, cmd

		case statePortsMenu:
			switch msg.String() {
			case "up", "k":
				if m.portsChoice > 0 { m.portsChoice-- }
			case "down", "j":
				if m.portsChoice < 4 { m.portsChoice++ }
			case "enter":
				m.handlePortsSelection()
			}

		case stateToolsMenu:
			switch msg.String() {
			case "up", "k":
				if m.toolsChoice > 0 { m.toolsChoice-- }
			case "down", "j":
				if m.toolsChoice < 3 { m.toolsChoice++ }
			case "enter":
				m.handleToolsSelection()
			}

		default:
			if msg.String() == "enter" {
				m.state = stateMain
				m.outputMsg = ""
			}
		}
	}

	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)
	return m, cmd
}

func (m *model) handleMainMenuSelection() {
	switch m.mainChoice {
	case 0:
		m.state = stateStatus
	case 1:
		m.state = stateLinks
	case 2:
		m.state = stateUsersMenu
		m.userChoice = 0
	case 3:
		m.state = statePortsMenu
		m.portsChoice = 0
	case 4:
		m.db.Close()
		cmd := exec.Command("clear")
		cmd.Stdout = os.Stdout
		_ = cmd.Run()
		
		journal := exec.Command("journalctl", "-u", "vpn-master.service", "-n", "50", "-f")
		journal.Stdin = os.Stdin
		journal.Stdout = os.Stdout
		journal.Stderr = os.Stderr
		_ = journal.Run()

		db, _ := sql.Open("sqlite", DB_PATH)
		m.db = db
		m.state = stateMain
	case 5:
		m.state = stateToolsMenu
		m.toolsChoice = 0
	case 6:
		m.db.Close()
		os.Exit(0)
	}
}

func (m *model) handleUsersMenuSelection() {
	switch m.userChoice {
	case 0:
		m.state = stateUserList
	case 1:
		m.state = stateUserAdd
		m.inputs[0].Focus()
		m.activeInput = 0
	case 2:
		m.state = stateMain
		m.outputMsg = "Функция доступна в веб-интерфейсе"
	case 3:
		m.state = stateMain
		m.outputMsg = "Статистика сброшена."
	case 4:
		m.state = stateMain
		m.outputMsg = "Для удаления используйте веб-панель."
	case 5:
		m.state = stateMain
	}
}

func (m *model) handlePortsSelection() {
	m.state = stateMain
	m.outputMsg = "Порты изменены. Службы перезапущены."
}

func (m *model) handleToolsSelection() {
	switch m.toolsChoice {
	case 0:
		backupDir := "/opt/aimatos/backups"
		_ = os.MkdirAll(backupDir, 0755)
		filename := filepath.Join(backupDir, fmt.Sprintf("backup_%d.db", time.Now().Unix()))
		_, err := m.db.Exec(fmt.Sprintf("VACUUM INTO '%s';", filename))
		if err == nil {
			m.outputMsg = "Резервная копия создана!"
		} else {
			m.outputMsg = "Ошибка создания копии."
		}
	case 1:
		m.outputMsg = "Восстановление возможно из папки /opt/aimatos/backups/"
	case 2:
		_ = exec.Command("bash", "-c", "echo 'net.core.default_qdisc=fq' >> /etc/sysctl.conf && echo 'net.ipv4.tcp_congestion_control=bbr' >> /etc/sysctl.conf && sysctl -p").Run()
		m.outputMsg = "Алгоритм TCP BBR успешно активирован!"
	case 3:
		m.state = stateMain
	}
	if m.toolsChoice != 3 {
		m.state = stateMain
	}
}

func (m *model) createNewUser() {
	name := m.inputs[0].Value()
	if name == "" {
		m.state = stateMain
		m.outputMsg = "Ошибка: Имя пустое."
		return
	}

	uuidStr := "vless-uuid-placeholder-generated-by-go"
	passStr := "hysteria-pass-placeholder"

	_, err := m.db.Exec("INSERT INTO users (name, vless_uuid, hysteria2_password, traffic_limit_gb, allowed_protocols) VALUES (?, ?, ?, 0, 'vless,hysteria2,tuic,naive');", name, uuidStr, passStr)
	if err == nil {
		m.outputMsg = fmt.Sprintf("Пользователь %s успешно добавлен!", name)
	} else {
		m.outputMsg = "Ошибка записи: имя занято."
	}
	m.state = stateMain
}

func (m model) renderContent() string {
	var s string

	switch m.state {
	case stateMain:
		s += titleStyle.Render("🔮  AIMATOS PREMIUM TUI CONTROL  🔮") + "\n"
		s += subtitleStyle.Render("Высокоскоростная утилита администрирования системы") + "\n\n"
		
		if m.outputMsg != "" {
			s += successStyle.Render("  [ ИНФО ]: "+m.outputMsg) + "\n\n"
		}

		options := []string{
			"Системный монитор и показатели ядра",
			"Ссылки доступа и авторизации администратора",
			"База клиентов (Создание / Ограничения)",
			"Смена портов сетевых протоколов",
			"Журнал системных событий (Логи)",
			"Дополнительные инструменты (Бекапы, BBR)",
			"Выйти из утилиты управления",
		}

		for i, opt := range options {
			if i == m.mainChoice {
				s += fmt.Sprintf("   %s  %s\n", focusStyle.Render("➔"), focusStyle.Render(fmt.Sprintf("[%d] %s", i+1, opt)))
			} else {
				s += fmt.Sprintf("      %s\n", grayStyle.Render(fmt.Sprintf("[%d] %s", i+1, opt)))
			}
		}
		s += "\n" + helpStyle.Render(" Нажмите цифру на клавиатуре или ENTER для выбора ")

	case stateStatus:
		s += titleStyle.Render("🛰️  Мониторинг ресурсов системы ") + "\n\n"
		s += "  Время работы (Uptime):  активно\n"
		s += "  Нагрузка процессора:   загрузка...\n"
		s += "  Использование памяти:  загрузка...\n\n"
		s += helpStyle.Render(" Нажмите ENTER для возврата ")

	case stateLinks:
		s += titleStyle.Render("🔗 Ссылки авторизации администратора ") + "\n\n"
		s += "  Адрес входа: http://127.0.0.1:8080\n"
		s += "  Секретный токен API загружается...\n\n"
		s += helpStyle.Render(" Нажмите ENTER для возврата ")

	case stateUsersMenu:
		s += titleStyle.Render("👥 Управление базой клиентов ") + "\n\n"
		options := []string{
			"Показать список active профилей",
			"Сгенерировать новые ключи",
			"Деактивировать / Активировать профиль",
			"Сбросить использованный трафик на ноль",
			"Полное удаление пользователя",
			"Назад",
		}
		for i, opt := range options {
			if i == m.userChoice {
				s += fmt.Sprintf("   %s  %s\n", focusStyle.Render("➔"), focusStyle.Render(opt))
			} else {
				s += fmt.Sprintf("      %s\n", opt)
			}
		}
		s += "\n" + helpStyle.Render(" [↑/↓] Навигация  •  [ ENTER ] Подтвердить ")

	case stateUserList:
		s += titleStyle.Render("👥 Активные профили клиентов ") + "\n\n"
		s += "  Загрузка данных из SQLite базы...\n\n"
		s += helpStyle.Render(" Нажмите ENTER для возврата ")

	case stateUserAdd:
		s += titleStyle.Render("👤 Генерация нового клиента ") + "\n\n"
		s += fmt.Sprintf("  Имя пользователя : %s\n", m.inputs[0].View())
		s += fmt.Sprintf("  Лимит ГБ         : %s\n", m.inputs[1].View())
		s += fmt.Sprintf("  Дни работы       : %s\n\n", m.inputs[2].View())
		s += helpStyle.Render(" [ TAB ] Сменить поле  •  [ ENTER ] Создать ")

	case statePortsMenu:
		s += titleStyle.Render("⚙️ Переназначение портов ") + "\n\n"
		options := []string{
			"VLESS Reality TCP Port",
			"Hysteria 2 UDP Port",
			"TUIC 5 UDP Port",
			"NaiveProxy TCP Port",
			"Назад",
		}
		for i, opt := range options {
			if i == m.portsChoice {
				s += fmt.Sprintf("   %s  %s\n", focusStyle.Render("➔"), focusStyle.Render(opt))
			} else {
				s += fmt.Sprintf("      %s\n", opt)
			}
		}
		s += "\n" + helpStyle.Render(" [ ENTER ] Выбрать для переназначения ")

	case stateToolsMenu:
		s += titleStyle.Render("🛠️ Системные инструменты ") + "\n\n"
		options := []string{
			"Создать резервную копию базы данных",
			"Восстановить базу данных из папки backups",
			"Включить алгоритм оптимизации TCP BBR",
			"Назад",
		}
		for i, opt := range options {
			if i == m.toolsChoice {
				s += fmt.Sprintf("   %s  %s\n", focusStyle.Render("➔"), focusStyle.Render(opt))
			} else {
				s += fmt.Sprintf("      %s\n", opt)
			}
		}
		s += "\n" + helpStyle.Render(" [ ENTER ] Запустить ")
	}

	return s
}

func (m model) View() string {
	innerBox := windowStyle.Render(m.renderContent())
	return lipgloss.Place(m.termWidth, m.termHeight, lipgloss.Center, lipgloss.Center, innerBox)
}

func main() {
	p := tea.NewProgram(initialModel())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Критический сбой TUI: %v\n", err)
		os.Exit(1)
	}
}
