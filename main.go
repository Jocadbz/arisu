package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/chzyer/readline"
)

type Message struct {
	Role    string
	Content string
}

type AIClient interface {
	SendMessage(input string) (string, error)
	AddMessage(role, content string)
	GetHistory() []Message
}

type Config struct {
	SelectedModel string            `json:"selected_model"`
	APIKeys       map[string]string `json:"api_keys"`
	AutoEdit      bool              `json:"auto_edit"`
	AutoRun       bool              `json:"auto_run"`
}

func loadConfig(configFile string) (*Config, error) {
	data, err := os.ReadFile(configFile)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{APIKeys: make(map[string]string)}, nil
		}
		return nil, err
	}
	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	if config.APIKeys == nil {
		config.APIKeys = make(map[string]string)
	}
	if config.SelectedModel == "grok" {
		config.SelectedModel = "grok-2-latest"
	}
	return &config, nil
}

func saveConfig(configFile string, config *Config) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(configFile), 0700); err != nil {
		return err
	}
	return os.WriteFile(configFile, data, 0600)
}

func logMessages(logFile string, history []Message, startIdx int) error {
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	for i := startIdx; i < len(history); i++ {
		msg := history[i]
		timestamp := time.Now().Format("2006-01-02 15:04:05")
		logEntry := fmt.Sprintf("[%s] %s: %s\n", timestamp, msg.Role, msg.Content)
		if _, err := f.WriteString(logEntry); err != nil {
			return err
		}
	}
	return nil
}

func main() {
	configDir := filepath.Join(os.Getenv("HOME"), ".config", "arisu")
	configFile := filepath.Join(configDir, "config.json")

	logDir := filepath.Join(configDir, "log")
	if err := os.MkdirAll(logDir, 0700); err != nil {
		fmt.Printf("Error creating log directory: %v\n", err)
		return
	}
	timestamp := time.Now().Format("20060102_150405")
	logFile := filepath.Join(logDir, "conversation_"+timestamp+".log")

	config, err := loadConfig(configFile)
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		return
	}

	args := os.Args[1:]
	if len(args) > 0 {
		switch args[0] {
		case "--setmodel":
			if len(args) < 2 {
				fmt.Println("Usage: arisu --setmodel <model>")
				return
			}
			model := args[1]
			if model == "grok" {
				model = "grok-2-latest"
			}
			config.SelectedModel = model
			if err := saveConfig(configFile, config); err != nil {
				fmt.Printf("Error saving config: %v\n", err)
				return
			}
			fmt.Printf("Selected model set to %s\n", model)
			return
		case "--auto-edit":
			if len(args) < 2 || (args[1] != "true" && args[1] != "false") {
				fmt.Println("Usage: arisu --auto-edit true/false")
				return
			}
			config.AutoEdit = args[1] == "true"
			if err := saveConfig(configFile, config); err != nil {
				fmt.Printf("Error saving config: %v\n", err)
				return
			}
			fmt.Printf("Auto-edit set to %v\n", config.AutoEdit)
			return
		case "--auto-run":
			if len(args) < 2 || (args[1] != "true" && args[1] != "false") {
				fmt.Println("Usage: arisu --auto-run true/false")
				return
			}
			config.AutoRun = args[1] == "true"
			if err := saveConfig(configFile, config); err != nil {
				fmt.Printf("Error saving config: %v\n", err)
				return
			}
			fmt.Printf("Auto-run set to %v\n", config.AutoRun)
			return
		}
	}

	if config.SelectedModel == "" {
		config.SelectedModel = "gemini"
	}

	openaiModels := []string{
		"gpt-4.1-mini",
		"gpt-4.1",
		"gpt-4o",
		"gpt-4o-mini",
		"o3",
		"gpt-3.5-turbo",
	}

	geminiModels := []string{
		"gemini",
		"gemini-2.0-flash",
		"gemini-2.5-flash",
		"gemini-2.5-pro",
	}

	var provider string
	if contains(geminiModels, config.SelectedModel) {
		provider = "gemini"
	} else if strings.HasPrefix(config.SelectedModel, "grok-") {
		provider = "grok"
	} else if contains(openaiModels, config.SelectedModel) {
		provider = "openai"
	} else if strings.HasPrefix(config.SelectedModel, "openrouter-") {
		provider = "openrouter"
	} else {
		fmt.Println("Invalid selected model in config.")
		return
	}

	apiKey, ok := config.APIKeys[provider]
	if !ok || apiKey == "" {
		fmt.Printf("Enter your %s API key: ", provider)
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			apiKey = scanner.Text()
		}
		if apiKey == "" {
			fmt.Println("Error: No API key provided.")
			return
		}
		config.APIKeys[provider] = apiKey
		if err := saveConfig(configFile, config); err != nil {
			fmt.Printf("Error saving config: %v\n", err)
		}
	}

	var client AIClient
	if provider == "gemini" {
		model := config.SelectedModel
		if model == "gemini" {
			model = "gemini-2.0-flash"
		}
		client = NewClient(apiKey, model)
	} else if provider == "grok" {
		client = NewGrokClient(apiKey, config.SelectedModel)
	} else if provider == "openai" {
		client = NewOpenAIClient(apiKey, config.SelectedModel)
	} else if provider == "openrouter" {
		client = NewOpenRouterClient(apiKey, config.SelectedModel)
	}

	if len(args) > 0 {
		prompt := strings.Join(args, " ")
		response, err := client.SendMessage(prompt)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return
		}

		for {
			output, isToolCall := handleResponse(response, client, config)
			_ = logMessages(logFile, client.GetHistory(), 0)

			if isToolCall {
				response, err = client.SendMessage(output)
				if err != nil {
					fmt.Printf("Error sending tool output: %v\n", err)
					return
				}
			} else {
				break
			}
		}
		return
	}

	fmt.Println("Enter text. Press Ctrl+D (EOF) on a new line to submit. Type 'exit' to quit.")
	rl, err := readline.New("λ ")
	if err != nil {
		fmt.Printf("Error initializing readline: %v\n", err)
		return
	}
	defer rl.Close()

	lastLoggedIndex := 0
	for {
		var inputLines []string
		for {
			if len(inputLines) > 0 {
				rl.SetPrompt(".. ")
			} else {
				rl.SetPrompt("λ ")
			}

			line, err := rl.Readline()
			if err == io.EOF {
				if len(inputLines) > 0 {
					break
				}
				return
			}
			if err == readline.ErrInterrupt {
				inputLines = nil
				continue
			}
			if err != nil {
				fmt.Printf("Error reading line: %v\n", err)
				return
			}

			if len(inputLines) == 0 && strings.TrimSpace(line) == "exit" {
				fmt.Println("Goodbye!")
				return
			}

			inputLines = append(inputLines, line)
		}
		if len(inputLines) == 0 {
			continue
		}
		input := strings.Join(inputLines, "\n")
		response, err := client.SendMessage(input)
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

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func confirmAction(prompt string) bool {
	fmt.Printf("%s (y/n): ", prompt)
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		return strings.ToLower(strings.TrimSpace(scanner.Text())) == "y"
	}
	return false
}

type Block struct {
	ID    int
	Lines []string
}

func parseBlocks(content string) []Block {
	lines := strings.Split(content, "\n")
	var blocks []Block
	var currentLines []string

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			if len(currentLines) > 0 {
				blocks = append(blocks, Block{ID: len(blocks), Lines: currentLines})
				currentLines = nil
			}
		} else {
			currentLines = append(currentLines, line)
		}
	}
	if len(currentLines) > 0 {
		blocks = append(blocks, Block{ID: len(blocks), Lines: currentLines})
	}
	return blocks
}

func blocksToString(blocks []Block) string {
	var sb strings.Builder
	for i, b := range blocks {
		for _, line := range b.Lines {
			sb.WriteString(line)
			sb.WriteString("\n")
		}
		if i < len(blocks)-1 {
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

type Action interface {
	Execute(client AIClient, config *Config, isToolCall bool) (string, error)
}

type PatchAction struct {
	Filename string
	ID       int
	Content  string
}

func (p PatchAction) Execute(client AIClient, config *Config, isToolCall bool) (string, error) {
	if config.AutoEdit || confirmAction(fmt.Sprintf("Apply patch to block %d in %s?", p.ID, p.Filename)) {
		content, err := os.ReadFile(p.Filename)
		if err != nil {
			fmt.Printf("Error reading %s: %v\n", p.Filename, err)
			return fmt.Sprintf("Error reading %s: %v", p.Filename, err), err
		}

		blocks := parseBlocks(string(content))
		if p.ID < 0 || p.ID >= len(blocks) {
			fmt.Printf("Error: Block ID %d not found in %s\n", p.ID, p.Filename)
			return fmt.Sprintf("Error: Block ID %d not found in %s", p.ID, p.Filename), fmt.Errorf("block id not found")
		}

		// Update block
		if strings.TrimSpace(p.Content) == "" {
			// Delete block
			blocks = append(blocks[:p.ID], blocks[p.ID+1:]...)
		} else {
			// Replace block
			newLines := strings.Split(p.Content, "\n")
			// Remove trailing newline from split if content ends with newline
			if len(newLines) > 0 && newLines[len(newLines)-1] == "" {
				newLines = newLines[:len(newLines)-1]
			}
			blocks[p.ID].Lines = newLines
		}

		newContent := blocksToString(blocks)
		if err := os.WriteFile(p.Filename, []byte(newContent), 0644); err != nil {
			fmt.Printf("Error writing %s: %v\n", p.Filename, err)
			return fmt.Sprintf("Error writing %s: %v", p.Filename, err), err
		}
		fmt.Printf("File %s patched successfully.\n", p.Filename)
		return fmt.Sprintf("File %s patched successfully.", p.Filename), nil
	} else {
		fmt.Printf("Patch on %s skipped.\n", p.Filename)
		return fmt.Sprintf("Patch on %s skipped.", p.Filename), nil
	}
}

type EditAction struct {
	Filename string
	Content  string
}

func (e EditAction) Execute(client AIClient, config *Config, isToolCall bool) (string, error) {
	if config.AutoEdit || confirmAction(fmt.Sprintf("Overwrite/Create %s?", e.Filename)) {
		if err := os.WriteFile(e.Filename, []byte(e.Content), 0644); err != nil {
			fmt.Printf("Error writing %s: %v\n", e.Filename, err)
			return fmt.Sprintf("Error writing %s: %v", e.Filename, err), err
		}
		fmt.Printf("File %s written successfully.\n", e.Filename)
		return fmt.Sprintf("File %s written successfully.", e.Filename), nil
	} else {
		fmt.Printf("Write on %s skipped.\n", e.Filename)
		return fmt.Sprintf("Write on %s skipped.", e.Filename), nil
	}
}

type RunAction struct {
	Command string
}

func (r RunAction) Execute(client AIClient, config *Config, isToolCall bool) (string, error) {
	if config.AutoRun || confirmAction(fmt.Sprintf("Execute command: %s?", r.Command)) {
		var outputBuf bytes.Buffer
		cmd := exec.Command("bash", "-c", r.Command)
		cmd.Stdout = io.MultiWriter(os.Stdout, &outputBuf)
		cmd.Stderr = io.MultiWriter(os.Stderr, &outputBuf)
		err := cmd.Run()
		if err != nil {
			fmt.Printf("Command failed with error: %v\n", err)
			return fmt.Sprintf("Command failed: %s\nError: %v", r.Command, err), err
		}
		output := outputBuf.String()
		if output != "" {
			return "Command output:\n" + output, nil
		}
		return "Command executed successfully (no output).", nil
	} else {
		fmt.Printf("Command skipped: %s\n", r.Command)
		return fmt.Sprintf("Command skipped: %s", r.Command), nil
	}
}

type ReadAction struct {
	Filename string
}

func (r ReadAction) Execute(client AIClient, config *Config, isToolCall bool) (string, error) {
	content, err := os.ReadFile(r.Filename)
	if err != nil {
		fmt.Printf("Error reading %s: %v\n", r.Filename, err)
		return fmt.Sprintf("Error reading %s: %v", r.Filename, err), err
	}

	blocks := parseBlocks(string(content))
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Content of %s (split into blocks):\n", r.Filename))
	for _, b := range blocks {
		sb.WriteString(fmt.Sprintf("--- BLOCK %d ---\n", b.ID))
		for _, line := range b.Lines {
			sb.WriteString(line)
			sb.WriteString("\n")
		}
	}

	fmt.Printf("Content of %s displayed in blocks.\n", r.Filename)
	return sb.String(), nil
}

func handleResponse(response string, client AIClient, config *Config) (string, bool) {
	var actions []struct {
		Action     Action
		IsToolCall bool
	}
	remainingResponse := response

	hasToolCall := false
	var outputBuilder strings.Builder

	for {
		patchStart := strings.Index(remainingResponse, "<PATCH>")
		editStart := strings.Index(remainingResponse, "<EDIT>")
		runStart := strings.Index(remainingResponse, "<RUN>")
		readStart := strings.Index(remainingResponse, "<READ>")

		if patchStart == -1 && editStart == -1 && runStart == -1 && readStart == -1 {
			break
		}

		type tagInfo struct {
			start int
			tag   string
		}

		firstTag := tagInfo{-1, ""}

		if patchStart != -1 {
			firstTag = tagInfo{patchStart, "PATCH"}
		}
		if editStart != -1 && (firstTag.start == -1 || editStart < firstTag.start) {
			firstTag = tagInfo{editStart, "EDIT"}
		}
		if runStart != -1 && (firstTag.start == -1 || runStart < firstTag.start) {
			firstTag = tagInfo{runStart, "RUN"}
		}
		if readStart != -1 && (firstTag.start == -1 || readStart < firstTag.start) {
			firstTag = tagInfo{readStart, "READ"}
		}

		// Check for [TOOL_CALL] prefix
		isToolCall := false
		prefix := remainingResponse[:firstTag.start]
		if strings.Contains(prefix, "[TOOL_CALL]") {
			// Simple check: if [TOOL_CALL] appears in the text before the tag,
			// and after the previous tag (which is handled by slicing remainingResponse).
			// However, we need to be careful about false positives if the user just typed it.
			// But the instruction is to prepend it.
			// Let's check if it's the last non-whitespace thing before the tag.
			trimmedPrefix := strings.TrimSpace(prefix)
			if strings.HasSuffix(trimmedPrefix, "[TOOL_CALL]") {
				isToolCall = true
				hasToolCall = true
			}
		}

		var endTag, content string
		var endIdx int

		switch firstTag.tag {
		case "PATCH":
			endTag = "</PATCH>"
			endIdx = strings.Index(remainingResponse, endTag)
			if endIdx == -1 {
				remainingResponse = remainingResponse[firstTag.start+len("<PATCH>"):]
				continue
			}
			content = remainingResponse[firstTag.start+len("<PATCH>") : endIdx]
			lines := strings.SplitN(strings.TrimSpace(content), "\n", 3)
			if len(lines) >= 2 {
				filename := strings.TrimSpace(lines[0])
				idStr := strings.TrimSpace(lines[1])
				id, err := strconv.Atoi(idStr)
				if err == nil {
					patchContent := ""
					if len(lines) == 3 {
						patchContent = lines[2]
					}
					actions = append(actions, struct {
						Action     Action
						IsToolCall bool
					}{PatchAction{Filename: filename, ID: id, Content: patchContent}, isToolCall})
				}
			}
		case "EDIT":
			endTag = "</EDIT>"
			endIdx = strings.Index(remainingResponse, endTag)
			if endIdx == -1 {
				remainingResponse = remainingResponse[firstTag.start+len("<EDIT>"):]
				continue
			}
			content = remainingResponse[firstTag.start+len("<EDIT>") : endIdx]
			lines := strings.SplitN(strings.TrimSpace(content), "\n", 2)
			if len(lines) == 2 {
				filename := strings.TrimSpace(lines[0])
				fileContent := lines[1]
				actions = append(actions, struct {
					Action     Action
					IsToolCall bool
				}{EditAction{Filename: filename, Content: fileContent}, isToolCall})
			}
		case "RUN":
			endTag = "</RUN>"
			endIdx = strings.Index(remainingResponse, endTag)
			if endIdx == -1 {
				remainingResponse = remainingResponse[firstTag.start+len("<RUN>"):]
				continue
			}
			content = remainingResponse[firstTag.start+len("<RUN>") : endIdx]
			actions = append(actions, struct {
				Action     Action
				IsToolCall bool
			}{RunAction{Command: strings.TrimSpace(content)}, isToolCall})
		case "READ":
			endTag = "</READ>"
			endIdx = strings.Index(remainingResponse, endTag)
			if endIdx == -1 {
				remainingResponse = remainingResponse[firstTag.start+len("<READ>"):]
				continue
			}
			content = remainingResponse[firstTag.start+len("<READ>") : endIdx]
			actions = append(actions, struct {
				Action     Action
				IsToolCall bool
			}{ReadAction{Filename: strings.TrimSpace(content)}, isToolCall})
		}

		if endIdx != -1 {
			remainingResponse = remainingResponse[endIdx+len(endTag):]
		}
	}

	for _, item := range actions {
		output, _ := item.Action.Execute(client, config, item.IsToolCall)
		if item.IsToolCall {
			outputBuilder.WriteString(output)
			outputBuilder.WriteString("\n")
		} else {
			client.AddMessage("user", output)
		}
	}

	return outputBuilder.String(), hasToolCall
}
