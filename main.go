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
	}

	if len(args) > 0 {
		prompt := strings.Join(args, " ")
		response, err := client.SendMessage(prompt)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return
		}
		handleResponse(response, client, config)
		checkAndRunAgenticMode(client, config, logDir)
		_ = logMessages(logFile, client.GetHistory(), 0)
		return
	}

	fmt.Println("For multi-line input, end with a blank line.")
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
			line, err := rl.Readline()
			if err == io.EOF {
				return
			}
			if err == readline.ErrInterrupt {
				continue
			}
			if err != nil {
				fmt.Printf("Error reading line: %v\n", err)
				return
			}
			line = strings.TrimSpace(line)
			if line == "exit" {
				fmt.Println("Goodbye!")
				return
			}
			if line == "" {
				break
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
		handleResponse(response, client, config)
		checkAndRunAgenticMode(client, config, logDir)
		_ = logMessages(logFile, client.GetHistory(), lastLoggedIndex)
		lastLoggedIndex = len(client.GetHistory())
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

type Action interface {
	Execute(client AIClient, config *Config) error
}

type EditAction struct {
	Filename string
	Content  string
}

func (e EditAction) Execute(client AIClient, config *Config) error {
	if config.AutoEdit || confirmAction(fmt.Sprintf("Apply edit to %s?", e.Filename)) {
		if err := os.WriteFile(e.Filename, []byte(e.Content), 0644); err != nil {
			fmt.Printf("Error editing %s: %v\n", e.Filename, err)
			client.AddMessage("user", fmt.Sprintf("Error editing %s: %v", e.Filename, err))
			return err
		}
		fmt.Printf("File %s edited successfully.\n", e.Filename)
		client.AddMessage("user", fmt.Sprintf("File %s edited:\n%s", e.Filename, e.Content))
	} else {
		fmt.Printf("Edit on %s skipped.\n", e.Filename)
	}
	return nil
}

type RunAction struct {
	Command string
}

func (r RunAction) Execute(client AIClient, config *Config) error {
	if config.AutoRun || confirmAction(fmt.Sprintf("Execute command: %s?", r.Command)) {
		var outputBuf bytes.Buffer
		cmd := exec.Command("bash", "-c", r.Command)
		cmd.Stdout = io.MultiWriter(os.Stdout, &outputBuf)
		cmd.Stderr = io.MultiWriter(os.Stderr, &outputBuf)
		err := cmd.Run()
		if err != nil {
			fmt.Printf("Command failed with error: %v\n", err)
			client.AddMessage("user", fmt.Sprintf("Command failed: %s\nError: %v", r.Command, err))
			return err
		}
		output := outputBuf.String()
		if output != "" {
			client.AddMessage("user", "Command output:\n"+output)
		}
	} else {
		fmt.Printf("Command skipped: %s\n", r.Command)
	}
	return nil
}

type ReadAction struct {
	Filename string
}

func (r ReadAction) Execute(client AIClient, config *Config) error {
	content, err := os.ReadFile(r.Filename)
	if err != nil {
		fmt.Printf("Error reading %s: %v\n", r.Filename, err)
		client.AddMessage("user", fmt.Sprintf("Error reading %s: %v", r.Filename, err))
		return err
	}
	fmt.Printf("Content of %s:\n%s\n", r.Filename, string(content))
	client.AddMessage("user", string(content))
	return nil
}

func handleResponse(response string, client AIClient, config *Config) {
	var actions []Action
	remainingResponse := response

	for {
		editStart := strings.Index(remainingResponse, "<EDIT>")
		runStart := strings.Index(remainingResponse, "<RUN>")
		readStart := strings.Index(remainingResponse, "<READ>")

		if editStart == -1 && runStart == -1 && readStart == -1 {
			break
		}

		type tagInfo struct {
			start int
			tag   string
		}

		firstTag := tagInfo{-1, ""}

		if editStart != -1 {
			firstTag = tagInfo{editStart, "EDIT"}
		}
		if runStart != -1 && (firstTag.start == -1 || runStart < firstTag.start) {
			firstTag = tagInfo{runStart, "RUN"}
		}
		if readStart != -1 && (firstTag.start == -1 || readStart < firstTag.start) {
			firstTag = tagInfo{readStart, "READ"}
		}

		var endTag, content string
		var endIdx int

		switch firstTag.tag {
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
				actions = append(actions, EditAction{Filename: filename, Content: fileContent})
			}
		case "RUN":
			endTag = "</RUN>"
			endIdx = strings.Index(remainingResponse, endTag)
			if endIdx == -1 {
				remainingResponse = remainingResponse[firstTag.start+len("<RUN>"):]
				continue
			}
			content = remainingResponse[firstTag.start+len("<RUN>") : endIdx]
			actions = append(actions, RunAction{Command: strings.TrimSpace(content)})
		case "READ":
			endTag = "</READ>"
			endIdx = strings.Index(remainingResponse, endTag)
			if endIdx == -1 {
				remainingResponse = remainingResponse[firstTag.start+len("<READ>"):]
				continue
			}
			content = remainingResponse[firstTag.start+len("<READ>") : endIdx]
			actions = append(actions, ReadAction{Filename: strings.TrimSpace(content)})
		}

		if endIdx != -1 {
			remainingResponse = remainingResponse[endIdx+len(endTag):]
		}
	}

	for _, action := range actions {
		_ = action.Execute(client, config)
	}
}

func checkAndRunAgenticMode(client AIClient, config *Config, logDir string) {
	agentFile := "AGENTSTEPS.arisu"
	if _, err := os.Stat(agentFile); os.IsNotExist(err) {
		return
	}
	runAgenticMode(agentFile, client, config, logDir)
}

func runAgenticMode(agentFile string, client AIClient, config *Config, logDir string) {
	data, err := os.ReadFile(agentFile)
	if err != nil {
		fmt.Printf("Error reading %s: %v\n", agentFile, err)
		return
	}

	content := string(data)
	parts := strings.Split(content, "Steps:")
	if len(parts) < 2 {
		fmt.Println("Invalid AGENTSTEPS.arisu file.")
		return
	}
	steps := strings.Split(strings.TrimSpace(parts[1]), "\n")
	agentLog := filepath.Join(logDir, "agent-run-"+time.Now().Format("20060102_150405")+".log")

	runCount := 0
	for _, step := range steps {
		step = strings.TrimSpace(step)
		if step == "" {
			continue
		}
		runCount++
		if runCount > 10 {
			fmt.Println("10-run limit reached. Exiting agentic mode.")
			client.AddMessage("assistant", "<END>")
			os.Remove("AGENTSTEPS.arisu")
			return
		}

		fmt.Printf("\n[Agentic] Executing step: %s\n", step)
		response, err := client.SendMessage(step)
		if err != nil {
			fmt.Printf("Error in agent run: %v\n", err)
			os.Remove("AGENTSTEPS.arisu")
			return
		}

		handleResponse(response, client, config)

		_ = appendToFile(agentLog, fmt.Sprintf("Step %d: %s\nResponse:\n%s\n\n", runCount, step, response))

		if strings.Contains(response, "<END>") {
			fmt.Println("[Agentic] Tag detected, exiting agentic mode.")
			os.Remove("AGENTSTEPS.arisu")
			return
		}

		proceedResponse, err := client.SendMessage("Proceed.")
		if err != nil {
			fmt.Printf("Error sending Proceed: %v\n", err)
			os.Remove("AGENTSTEPS.arisu")
			return
		}

		handleResponse(proceedResponse, client, config)

		_ = appendToFile(agentLog, fmt.Sprintf("Proceed Response for Step %d:\n%s\n\n", runCount, proceedResponse))
	}
}

func appendToFile(filename, text string) error {
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(text)
	return err
}
