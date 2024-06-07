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
	"sync"
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
	// host = "0.0.0.0"
	host = "localhost"
	port = "23234"
)

var pieces = map[int]rune{
	1:  '○',
	-1: '×',
	2:  '-',
	3:  '|',
	4:  '\\',
	5:  '/',
	0:  ' ',
}

type player struct {
	name      string
	score     int
	txtStyle  lipgloss.Style
	quitStyle lipgloss.Style
	term      string
	width     int
	height    int
	bg        string
	ch        chan tea.Msg
}

type model struct {
	board         [][]int
	currentPlayer int
	view          int
	textInput     textinput.Model
	players       [2]player
}

type gameState struct {
	players  [2]*ssh.Session
	mu       sync.Mutex
	m        model
	sessions map[string]chan tea.Msg
}

var state = gameState{
	m:        newBubbleteaModel(),
	sessions: make(map[string]chan tea.Msg),
}

func newBubbleteaModel() model {
	// initialize tea model
	ti := textinput.New()
	ti.Focus()
	ti.CharLimit = 20
	ti.Width = 20
	return model{
		view:          1,
		currentPlayer: 1,
		textInput:     ti,
		board: [][]int{
			{0, 0, 0},
			{0, 0, 0},
			{0, 0, 0},
		},
	}
}

func main() {
	// start app server
	s, err := wish.NewServer(
		wish.WithAddress(net.JoinHostPort(host, port)),
		wish.WithHostKeyPath(".ssh/id_ed25519"),
		wish.WithMiddleware(
			bubbletea.Middleware(teaHandler),
			// gameHandler(),
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

// RegisterSession registers a new session to receive updates.
func (gs *gameState) RegisterSession(id string, ch chan tea.Msg) {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	gs.sessions[id] = ch
	if gs.m.players[0].ch == nil {
		gs.m.players[0].ch = ch
	} else {
		gs.m.players[1].ch = ch
	}

	go func() {
		for {
			<-ch
			state.BroadcastMessage(redraw)
		}
	}()
}

// UnregisterSession removes a session from receiving updates.
func (gs *gameState) UnregisterSession(id string) {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	delete(gs.sessions, id)
}

// BroadcastMessage sends a message to all registered sessions.
func (gs *gameState) BroadcastMessage(msg tea.Msg) {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	for _, ch := range gs.sessions {
		ch <- msg
	}
}

// UpdateModel updates the global model and broadcasts the change.
// func (gs *gameState) UpdateModel(msg tea.Msg) {
// 	gs.mu.Lock()
// 	defer gs.mu.Unlock()
// 	m, _ := gs.m.Update(msg)
// 	gs.m = m.(model)
// 	gs.BroadcastMessage(msg)
// }

// // You can wire any Bubble Tea model up to the middleware with a function that
// // handles the incoming ssh.Session. Here we just grab the terminal info and
// // pass it to the new model. You can also return tea.ProgramOptions (such as
// // tea.WithAltScreen) on a session by session basis.
// func gameHandler() wish.Middleware {
// 	// sessionID := s.Context().Value(ssh.ContextKeySessionID).(string)
// 	// msgCh := make(chan tea.Msg)
// 	// state.RegisterSession(sessionID, msgCh)
// 	// defer state.UnregisterSession(sessionID)

// 	// Initialize bubbletea program for this session.
// 	p := tea.NewProgram(state.m)
// 	// Goroutine to listen for global state updates and send them to the session's program.
// 	// go func() {
// 	// 	for msg := range msgCh {
// 	// 		p.Send(msg)
// 	// 	}
// 	// }()
// 	return bubbletea.MiddlewareWithProgramHandler(teaHandler, termenv.ANSI256)
// 	// Start the bubbletea program.
// 	if _, err := p.Run(); err != nil {
// 		fmt.Println("Error:", err)
// 	}

// }

func teaHandler(s ssh.Session) (tea.Model, []tea.ProgramOption) {
	// This should never fail, as we are using the activeterm middleware.
	log.Info("debug", "len", cap(state.players))

	sessionID := s.Context().Value(ssh.ContextKeySessionID).(string)
	msgCh := make(chan tea.Msg)
	state.RegisterSession(sessionID, msgCh)
	defer state.UnregisterSession(sessionID)

	// Manage user sessions
	if state.players[0] == nil {
		state.players[0] = &s
		pty, _, _ := s.Pty()
		renderer := bubbletea.MakeRenderer(s)
		state.m.players[0].txtStyle = renderer.NewStyle().Foreground(lipgloss.Color("10"))
		state.m.players[0].quitStyle = renderer.NewStyle().Foreground(lipgloss.Color("8"))
		state.m.players[0].bg = "light"
		if renderer.HasDarkBackground() {
			state.m.players[0].bg = "dark"
		}
		state.m.players[0].name = s.User()
		state.m.players[0].term = pty.Term
		state.m.players[0].width = pty.Window.Width
		state.m.players[0].height = pty.Window.Height
		log.Info("Connected player 1:", "name", s.User())
	} else if state.players[1] == nil {
		state.players[1] = &s
		pty, _, _ := s.Pty()
		renderer := bubbletea.MakeRenderer(s)
		state.m.players[1].txtStyle = renderer.NewStyle().Foreground(lipgloss.Color("10"))
		state.m.players[1].quitStyle = renderer.NewStyle().Foreground(lipgloss.Color("8"))
		state.m.players[1].bg = "light"
		if renderer.HasDarkBackground() {
			state.m.players[1].bg = "dark"
		}
		state.m.players[1].name = s.User()
		state.m.players[1].term = pty.Term
		state.m.players[1].width = pty.Window.Width
		state.m.players[1].height = pty.Window.Height
		log.Info("Connected player 2:", "name", s.User())
	} else {
		s.Close()
	}
	return state.m, []tea.ProgramOption{tea.WithAltScreen()}
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
			m.players[0].score++
		} else {
			m.players[1].score++
		}
	}
}

type redrawMsg string

func redraw() tea.Msg {
	return redrawMsg("")
}

// ---------- Bubbletea functions -------------
func (m model) Init() tea.Cmd {
	// return textinput.Blink
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case redrawMsg:
		m.players[0].txtStyle.Render(m.View())
		return m, nil
	case tea.WindowSizeMsg:
		m.players[0].height = msg.Height
		m.players[0].width = msg.Width
	case tea.KeyMsg:
		switch m.view {
		case 0:
			switch msg.String() {
			case "enter":
				switch m.currentPlayer {
				case 1:
					m.players[0].name = m.textInput.Value()
					m.textInput.Placeholder = "Player 2 name?"
					m.textInput.Reset()
					m.currentPlayer *= -1
				case -1:
					m.players[1].name = m.textInput.Value()
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
			// state.BroadcastMessage(redraw)
			// m.players[0].ch <- "0"
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
			m.players[0].name,
			m.players[0].score,
			m.players[1].name,
			m.players[1].score,
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
