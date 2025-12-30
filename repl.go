package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

type errMsg error

type model struct {
	textarea textarea.Model
	err      error
	input    string
	quitting bool
	aborted  bool
}

func initialModel() model {
	ti := textarea.New()
	ti.Placeholder = "Ask Arisu... (Enter to send, Ctrl+N/Alt+Enter for new line, Ctrl+E for editor)"
	ti.Focus()

	ti.Prompt = "λ "
	ti.CharLimit = 0 // Unlimited
	ti.SetWidth(80)
	ti.SetHeight(3)

	// Remove default keybindings that might conflict if we want custom handling
	// But textarea default is Enter -> Newline.
	// We want Enter -> Submit, Alt+Enter -> Newline.
	ti.KeyMap.InsertNewline.SetEnabled(false)

	return model{
		textarea: ti,
		err:      nil,
	}
}

func (m model) Init() tea.Cmd {
	return textarea.Blink
}

func openEditor(initialContent string) (string, error) {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vim"
	}

	tmpfile, err := os.CreateTemp("", "arisu-buffer-*.md")
	if err != nil {
		return initialContent, err
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.WriteString(initialContent); err != nil {
		return initialContent, err
	}
	if err := tmpfile.Close(); err != nil {
		return initialContent, err
	}

	cmd := exec.Command(editor, tmpfile.Name())
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return initialContent, err
	}

	content, err := os.ReadFile(tmpfile.Name())
	if err != nil {
		return initialContent, err
	}

	return string(content), nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			m.quitting = true
			m.aborted = true
			return m, tea.Quit
		case tea.KeyCtrlE:
			// Open external editor
			newContent, err := openEditor(m.textarea.Value())
			if err == nil {
				m.textarea.SetValue(newContent)
				// Move cursor to end
				// textarea.Model doesn't expose Cursor.SetPosition directly in older versions or it's different.
				// But SetValue usually resets cursor or keeps it.
				// Let's just leave it, or use m.textarea.CursorEnd() if available.
				// Actually, SetValue puts cursor at the end usually.
			}
			return m, nil
		case tea.KeyEnter:
			// Check for Alt+Enter or just Enter
			// Bubble Tea's key.Msg has Alt bool.
			if msg.Alt {
				m.textarea.InsertString("\n")
				return m, nil
			}
			
			// Standard Enter -> Submit
			m.input = m.textarea.Value()
			m.quitting = true
			return m, tea.Quit
		case tea.KeyCtrlD:
			// EOF behavior
			if m.textarea.Value() == "" {
				m.quitting = true
				m.aborted = true
				return m, tea.Quit
			}
		case tea.KeyCtrlN, tea.KeyCtrlJ:
			// Explicit newline
			m.textarea.InsertString("\n")
			return m, nil
		}

	case errMsg:
		m.err = msg
		return m, nil
	}

	m.textarea, cmd = m.textarea.Update(msg)
	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	if m.quitting {
		return ""
	}
	return fmt.Sprintf(
		"%s\n\n%s",
		m.textarea.View(),
		"(Enter to send, Ctrl+N/Alt+Enter for new line, Ctrl+E for editor)",
	) + "\n"
}

// StartREPL starts the Bubble Tea input loop
func StartREPL(client AIClient, config *Config, logFile string) {
	fmt.Println("Welcome to Arisu. Type 'exit' to quit.")
	
	lastLoggedIndex := 0

	for {
		p := tea.NewProgram(initialModel())
		m, err := p.Run()
		if err != nil {
			fmt.Printf("Error running program: %v\n", err)
			return
		}

		finalModel := m.(model)
		if finalModel.aborted {
			fmt.Println("Goodbye!")
			return
		}

		input := strings.TrimSpace(finalModel.input)
		if input == "" {
			continue
		}

		if input == "exit" {
			fmt.Println("Goodbye!")
			return
		}

		// Print the user's input to stdout so it remains in history
		// (Bubble Tea clears the view on exit usually, or we can make it persistent)
		// Since we returned "", the view is cleared. We should print the prompt and input.
		fmt.Printf("λ %s\n", input)

		// Process @ mentions
		words := strings.Fields(input)
		finalInput := input
		for _, word := range words {
			if strings.HasPrefix(word, "@") {
				filename := strings.TrimPrefix(word, "@")
				content, err := os.ReadFile(filename)
				if err == nil {
					fileBlock := fmt.Sprintf("\n<FILE name=\"%s\">\n%s\n</FILE>\n", filename, string(content))
					finalInput = strings.Replace(finalInput, word, fileBlock, 1)
				}
			}
		}

		response, err := client.SendMessage(finalInput)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}

		for {
			output, isToolCall := handleResponse(response, client, config)
			_ = logMessages(logFile, client.GetHistory(), lastLoggedIndex)
			lastLoggedIndex = len(client.GetHistory())

			if isToolCall {
				response, err = client.SendMessage(output)
				if err != nil {
					fmt.Printf("Error sending tool output: %v\n", err)
					break
				}
			} else {
				break
			}
		}
	}
}
