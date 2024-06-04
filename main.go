package main

// An example Bubble Tea server. This will put an ssh session into alt screen
// and continually print up to date terminal information.

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	"github.com/charmbracelet/wish/activeterm"
	"github.com/charmbracelet/wish/bubbletea"
	"github.com/charmbracelet/wish/logging"
)

const (
	host = "0.0.0.0"
	port = "23234"
)

func main() {
	s, err := wish.NewServer(
		wish.WithAddress(net.JoinHostPort(host, port)),
		wish.WithHostKeyPath(".ssh/id_ed25519"),
		wish.WithMiddleware(
			bubbletea.Middleware(teaHandler),
			activeterm.Middleware(), // Bubble Tea apps usually require a PTY.
			logging.Middleware(),
		),
	)
	if err != nil {
		log.Error("Could not start server", "error", err)
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	log.Info("Starting SSH server", "host", host, "port", port)
	go func() {
		if err = s.ListenAndServe(); err != nil && !errors.Is(err, ssh.ErrServerClosed) {
			log.Error("Could not start server", "error", err)
			done <- nil
		}
	}()

	<-done
	log.Info("Stopping SSH server")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer func() { cancel() }()
	if err := s.Shutdown(ctx); err != nil && !errors.Is(err, ssh.ErrServerClosed) {
		log.Error("Could not stop server", "error", err)
	}
}

// You can wire any Bubble Tea model up to the middleware with a function that
// handles the incoming ssh.Session. Here we just grab the terminal info and
// pass it to the new model. You can also return tea.ProgramOptions (such as
// tea.WithAltScreen) on a session by session basis.
func teaHandler(s ssh.Session) (tea.Model, []tea.ProgramOption) {
	// This should never fail, as we are using the activeterm middleware.
	pty, _, _ := s.Pty()

	// When running a Bubble Tea app over SSH, you shouldn't use the default
	// lipgloss.NewStyle function.
	// That function will use the color profile from the os.Stdin, which is the
	// server, not the client.
	// We provide a MakeRenderer function in the bubbletea middleware package,
	// so you can easily get the correct renderer for the current session, and
	// use it to create the styles.
	// The recommended way to use these styles is to then pass them down to
	// your Bubble Tea model.
	renderer := bubbletea.MakeRenderer(s)
	txtStyle := renderer.NewStyle().Foreground(lipgloss.Color("10"))
	quitStyle := renderer.NewStyle().Foreground(lipgloss.Color("8"))

	bg := "light"
	if renderer.HasDarkBackground() {
		bg = "dark"
	}

	ti := textinput.New()
	ti.Placeholder = "Player 1 name?"
	ti.Focus()
	ti.CharLimit = 20
	ti.Width = 20
	m := model{
		view:          0,
		currentPlayer: 1,
		textInput:     ti,
		board: [][]int{
			{0, 0, 0},
			{0, 0, 0},
			{0, 0, 0},
		},
		term:      pty.Term,
		width:     pty.Window.Width,
		height:    pty.Window.Height,
		bg:        bg,
		txtStyle:  txtStyle,
		quitStyle: quitStyle,
	}
	return m, []tea.ProgramOption{tea.WithAltScreen()}
}

// Just a generic tea.Model to demo terminal information of ssh.
type model struct {
	board         [][]int
	currentPlayer int
	player1_name  string
	player1_score int
	player2_name  string
	player2_score int
	view          int
	textInput     textinput.Model
	txtStyle      lipgloss.Style
	quitStyle     lipgloss.Style
	term          string
	width         int
	height        int
	bg            string
}

var pieces = map[int]rune{
	1:  '○',
	-1: '×',
	2:  '-',
	3:  '|',
	4:  '\\',
	5:  '/',
	0:  ' ',
}

func updateCell(m *model, x int, y int) {
	var cell = &m.board[x][y]
	if *cell == 0 {
		*cell = m.currentPlayer
		m.currentPlayer *= -1
	} else if *cell == 1 || *cell == -1 {
		*cell *= -1
	}
	// check if row is the same player
	var victory = false
	if m.board[x][0] == m.board[x][1] && m.board[x][1] == m.board[x][2] {
		m.board[x][0] = 2
		m.board[x][1] = 2
		m.board[x][2] = 2
		victory = true
	}
	// check if column is the same player
	if m.board[0][y] == m.board[1][y] && m.board[1][y] == m.board[2][y] {
		m.board[0][y] = 3
		m.board[1][y] = 3
		m.board[2][y] = 3
		victory = true
	}
	// check if diagonal is the same player
	if m.board[0][0] == m.board[1][1] && m.board[1][1] == m.board[2][2] {
		if m.board[1][1] == 1 || m.board[1][1] == -1 {
			m.board[0][0] = 4
			m.board[1][1] = 4
			m.board[2][2] = 4
			victory = true
		}
	}
	// Check secondary diagonal
	if m.board[0][2] == m.board[1][1] && m.board[1][1] == m.board[2][0] {
		if m.board[1][1] == 1 || m.board[1][1] == -1 {
			m.board[0][2] = 5
			m.board[1][1] = 5
			m.board[2][0] = 5
			victory = true
		}
	}
	if victory {
		if m.currentPlayer != 1 {
			m.player1_score++
		} else {
			m.player2_score++
		}
	}
}

// ---------- Bubbletea functions -------------
func (m model) Init() tea.Cmd {
	return textinput.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height
		m.width = msg.Width
	case tea.KeyMsg:
		switch m.view {
		case 0:
			switch msg.String() {
			case "enter":
				switch m.currentPlayer {
				case 1:
					m.player1_name = m.textInput.Value()
					m.textInput.Placeholder = "Player 2 name?"
					m.textInput.Reset()
					m.currentPlayer *= -1
				case -1:
					m.player2_name = m.textInput.Value()
					m.currentPlayer *= -1
					m.view = 1
				}
			default:
				var cmd tea.Cmd
				m.textInput, cmd = m.textInput.Update(msg)
				return m, cmd
			}
		case 1:
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "q":
				updateCell(&m, 0, 0)
			case "w":
				updateCell(&m, 0, 1)
			case "e":
				updateCell(&m, 0, 2)
			case "a":
				updateCell(&m, 1, 0)
			case "s":
				updateCell(&m, 1, 1)
			case "d":
				updateCell(&m, 1, 2)
			case "z":
				updateCell(&m, 2, 0)
			case "x":
				updateCell(&m, 2, 1)
			case "c":
				updateCell(&m, 2, 2)
			case "0":
				m.view = 0
			case "1":
				m.view = 1
			case "2":
				m.view = 2
			case "esc":
				m.board = [][]int{
					{0, 0, 0},
					{0, 0, 0},
					{0, 0, 0},
				}
			}
			return m, nil
		}
	}
	return m, nil
}

//	func (m model) View() string {
//		s := fmt.Sprintf("Your term is %s\nYour window size is %dx%d\nBackground: %s\n", m.term, m.width, m.height, m.bg)
//		return m.txtStyle.Render(s) + "\n\n" + m.quitStyle.Render("Press 'q' to quit\n")
//	}
func (m model) View() string {
	v := "Tik-Tag-Go"
	switch m.view {
	case 0:
		v = m.textInput.View()
	case 1:
		v = fmt.Sprintf("%s: %d\n%s: %d\n┏━┳━┳━┓\n┃%c┃%c┃%c┃\n┣━╋━╋━┫\n┃%c┃%c┃%c┃\n┣━╋━╋━┫\n┃%c┃%c┃%c┃\n┗━┻━┻━┛",
			m.player1_name,
			m.player1_score,
			m.player2_name,
			m.player2_score,
			pieces[m.board[0][0]],
			pieces[m.board[0][1]],
			pieces[m.board[0][2]],
			pieces[m.board[1][0]],
			pieces[m.board[1][1]],
			pieces[m.board[1][2]],
			pieces[m.board[2][0]],
			pieces[m.board[2][1]],
			pieces[m.board[2][2]])
	}
	return v
}
