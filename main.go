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
)

// Message represents a single message in the conversation history.
type Message struct {
    Role    string
    Content string
}

// AIClient defines the interface for AI clients (Gemini and Grok).
type AIClient interface {
    SendMessage(input string) (string, error)
    AddMessage(role, content string)
    GetHistory() []Message
}

// Config holds the configuration including the selected model, API keys, and auto options.
type Config struct {
    SelectedModel string            `json:"selected_model"`
    APIKeys       map[string]string `json:"api_keys"`
    AutoEdit      bool              `json:"auto_edit"`
    AutoRun       bool              `json:"auto_run"`
}

// loadConfig loads the configuration from the config file.
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
    // Map "grok" to "grok-2-latest" for backward compatibility
    if config.SelectedModel == "grok" {
        config.SelectedModel = "grok-2-latest"
    }
    return &config, nil
}

// saveConfig saves the configuration to the config file.
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

// logMessages appends new messages from the chat history to the log file.
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

    // Set up log directory and file
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

    // Default to Gemini if no model is selected
    if config.SelectedModel == "" {
        config.SelectedModel = "gemini"
    }

    var provider string
    if config.SelectedModel == "gemini" {
        provider = "gemini"
    } else if strings.HasPrefix(config.SelectedModel, "grok-") {
        provider = "grok"
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
        client = NewClient(apiKey)
    } else if provider == "grok" {
        client = NewGrokClient(apiKey, config.SelectedModel)
    }

    if len(args) > 0 {
        prompt := strings.Join(args, " ")
        response, err := client.SendMessage(prompt)
        if err != nil {
            fmt.Printf("Error: %v\n", err)
            return
        }
        handleResponse(response, client, config)
        if err := logMessages(logFile, client.GetHistory(), 0); err != nil {
            fmt.Printf("Error logging messages: %v\n", err)
        }
        return
    }

    fmt.Println("For multi-line input, end with a blank line.")
    scanner := bufio.NewScanner(os.Stdin)
    lastLoggedIndex := 0
    for {
        fmt.Print("λ ")
        var inputLines []string

        for {
            if !scanner.Scan() {
                return
            }
            line := scanner.Text()

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
        if err := logMessages(logFile, client.GetHistory(), lastLoggedIndex); err != nil {
            fmt.Printf("Error logging messages: %v\n", err)
        }
        lastLoggedIndex = len(client.GetHistory())
    }
}

// confirmAction prompts the user for confirmation and returns true if confirmed.
func confirmAction(prompt string) bool {
    fmt.Printf("%s (y/n): ", prompt)
    scanner := bufio.NewScanner(os.Stdin)
    if scanner.Scan() {
        return strings.ToLower(strings.TrimSpace(scanner.Text())) == "y"
    }
    return false
}

// handleResponse processes the AI's response, handling commands and edits with confirmation.
func handleResponse(response string, client AIClient, config *Config) {
    editRequests := extractEditRequests(response)
    commands := extractCommands(response)
    readRequests := extractReadRequests(response)

    // Handle file edits first
    for _, req := range editRequests {
        if config.AutoEdit || confirmAction(fmt.Sprintf("Apply edit to %s?", req.Filename)) {
            if err := os.WriteFile(req.Filename, []byte(req.Content), 0644); err != nil {
                fmt.Printf("Erro ao editar %s: %v\n", req.Filename, err)
                client.AddMessage("user", fmt.Sprintf("Erro ao editar %s: %v", req.Filename, err))
            } else {
                fmt.Printf("Arquivo %s editado com sucesso.\n", req.Filename)
                client.AddMessage("user", fmt.Sprintf("Arquivo %s editado:\n%s", req.Filename, req.Content))
            }
        } else {
            fmt.Printf("Edit to %s skipped.\n", req.Filename)
        }
    }

    // Then handle commands
    for _, command := range commands {
        if config.AutoRun || confirmAction(fmt.Sprintf("Run command: %s?", command)) {
            var outputBuf bytes.Buffer
            cmd := exec.Command("bash", "-c", command)
            cmd.Stdout = io.MultiWriter(os.Stdout, &outputBuf)
            cmd.Stderr = io.MultiWriter(os.Stderr, &outputBuf)
            err := cmd.Run()
            if err != nil {
                fmt.Printf("Command failed with error: %v\n", err)
            }
            output := outputBuf.String()
            if output != "" {
                client.AddMessage("user", "Command output:\n"+output)
            }
        } else {
            fmt.Printf("Command skipped: %s\n", command)
        }
    }

    // Handle read requests
    for _, filename := range readRequests {
        content, err := os.ReadFile(filename)
        if err != nil {
            fmt.Printf("Erro ao ler %s: %v\n", filename, err)
            client.AddMessage("user", fmt.Sprintf("Erro ao ler %s: %v", filename, err))
        } else {
            fmt.Printf("Conteúdo de %s:\n%s\n", filename, string(content))
            client.AddMessage("user", fmt.Sprintf("Conteúdo de %s:\n%s", filename, string(content)))
        }
    }
}

// extractCommands extracts bash commands between <RUN> and </RUN>.
func extractCommands(response string) []string {
    var commands []string
    start := 0
    for {
        startIdx := strings.Index(response[start:], "<RUN>")
        if startIdx == -1 {
            break
        }
        startIdx += start
        endIdx := strings.Index(response[startIdx:], "</RUN>")
        if endIdx == -1 {
            break
        }
        endIdx += startIdx
        command := strings.TrimSpace(response[startIdx+5 : endIdx])
        commands = append(commands, command)
        start = endIdx + 6
    }
    return commands
}

// extractReadRequests extracts read requests between <READ> and </READ>.
func extractReadRequests(response string) []string {
    var filenames []string
    start := 0
    for {
        startIdx := strings.Index(response[start:], "<READ>")
        if startIdx == -1 {
            break
        }
        startIdx += start
        endIdx := strings.Index(response[startIdx:], "</READ>")
        if endIdx == -1 {
            break
        }
        endIdx += startIdx
        filename := strings.TrimSpace(response[startIdx+6 : endIdx])
        filenames = append(filenames, filename)
        start = endIdx + 7
    }
    return filenames
}

// EditRequest represents an edit request with filename and content.
type EditRequest struct {
    Filename string
    Content  string
}

// extractEditRequests extracts edit requests between <EDIT> and </EDIT>.
func extractEditRequests(response string) []EditRequest {
    var requests []EditRequest
    start := 0
    for {
        startIdx := strings.Index(response[start:], "<EDIT>")
        if startIdx == -1 {
            break
        }
        startIdx += start
        endIdx := strings.Index(response[startIdx:], "</EDIT>")
        if endIdx == -1 {
            break
        }
        endIdx += startIdx
        content := strings.TrimSpace(response[startIdx+6 : endIdx])
        lines := strings.SplitN(content, "\n", 2)
        if len(lines) < 2 {
            start = endIdx + 7
            continue
        }
        filename := strings.TrimSpace(lines[0])
        editContent := strings.TrimSpace(lines[1])
        requests = append(requests, EditRequest{Filename: filename, Content: editContent})
        start = endIdx + 7
    }
    return requests
}
