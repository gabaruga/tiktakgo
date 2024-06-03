package main

// A simple program that counts down from 5 and then exits.

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	// Log to a file. Useful in debugging since you can't really log to stdout.
	// Not required.
	logfilePath := os.Getenv("BUBBLETEA_LOG")
	if logfilePath != "" {
		if _, err := tea.LogToFile(logfilePath, "simple"); err != nil {
			log.Fatal(err)
		}
	}

	// Initialize our program
	ti := textinput.New()
	ti.Placeholder = "Player 1 name?"
	ti.Focus()
	ti.CharLimit = 20
	ti.Width = 20

	p := tea.NewProgram(model{
		tick:          0,
		view:          0,
		currentPlayer: 1,
		textInput:     ti,
		board: [][]int{
			{0, 0, 0},
			{0, 0, 0},
			{0, 0, 0},
		},
	}, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}
}

// A model can be more or less any type of data. It holds all the data for a
// program, so often it's a struct. For this simple example, however, all
// we'll need is a simple integer.
type model struct {
	tick          int
	board         [][]int
	currentPlayer int
	player1_name  string
	player1_score int
	player2_name  string
	player2_score int
	view          int
	textInput     textinput.Model
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

// Init optionally returns an initial command we should run. In this case we
// want to start the timer.
func (m model) Init() tea.Cmd {
	if m.tick > 0 {
		return tick
	}

	if m.view == 0 {
		return textinput.Blink
	}

	return nil
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

// Update is called when messages are received. The idea is that you inspect the
// message and send back an updated model accordingly. You can also return
// a command, which is a function that performs I/O and returns a message.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
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
	case tickMsg:
		m.tick--
		if m.tick <= 0 {
			return m, tea.Quit
		}
		return m, tick
	}
	return m, nil
}

// View returns a string based on data in the model. That string which will be
// rendered to the terminal.
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

// Messages are events that we respond to in our Update function. This
// particular one indicates that the timer has ticked.
type tickMsg time.Time

func tick() tea.Msg {
	time.Sleep(time.Second)
	return tickMsg{}
}
