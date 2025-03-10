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
    "time" // Added for timestamp generation

    "github.com/google/generative-ai-go/genai" // Required for genai.Content type
)

// logMessages appends new messages from the chat history to the log file starting from startIdx.
func logMessages(logFile string, history []*genai.Content, startIdx int) error {
    f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
    if err != nil {
        return err
    }
    defer f.Close()

    for i := startIdx; i < len(history); i++ {
        msg := history[i]
        timestamp := time.Now().Format("2006-01-02 15:04:05")
        role := msg.Role
        content := ""
        for _, part := range msg.Parts {
            if text, ok := part.(genai.Text); ok {
                content += string(text)
            }
        }
        logEntry := fmt.Sprintf("[%s] %s: %s\n", timestamp, role, content)
        if _, err := f.WriteString(logEntry); err != nil {
            return err
        }
    }
    return nil
}

// main é o ponto de entrada do programa.
func main() {
    configDir := filepath.Join(os.Getenv("HOME"), ".config", "arisu")
    configFile := filepath.Join(configDir, "config.json")

    // Set up log directory and file
    logDir := filepath.Join(configDir, "log")
    if err := os.MkdirAll(logDir, 0700); err != nil {
        fmt.Printf("Error creating log directory: %v\n", err)
        return
    }
    timestamp := time.Now().Format("20060102_150405") // Format: YYYYMMDD_HHMMSS
    logFile := filepath.Join(logDir, "conversation_"+timestamp+".log")

    apiKey := loadAPIKey(configFile)
    if apiKey == "" {
        fmt.Print("Enter your Gemini API key: ")
        scanner := bufio.NewScanner(os.Stdin)
        if scanner.Scan() {
            apiKey = scanner.Text()
        }
        if apiKey == "" {
            fmt.Println("Error: No API key provided.")
            return
        }
        saveAPIKey(configFile, apiKey)
    }

    client := NewClient(apiKey)

    args := os.Args[1:]
    if len(args) > 0 {
        prompt := strings.Join(args, " ")
        response, err := client.SendMessage(prompt)
        if err != nil {
            fmt.Printf("Error: %v\n", err)
            return
        }
        handleResponse(response, client)
        // Log the entire conversation history for single command
        if err := logMessages(logFile, client.cs.History, 0); err != nil {
            fmt.Printf("Error logging messages: %v\n", err)
        }
        return
    }

    fmt.Println("For multi-line input, end with a blank line.")
    scanner := bufio.NewScanner(os.Stdin)
    lastLoggedIndex := 0 // Track the last logged message index
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
        handleResponse(response, client)
        // Log new messages added to the history
        if err := logMessages(logFile, client.cs.History, lastLoggedIndex); err != nil {
            fmt.Printf("Error logging messages: %v\n", err)
        }
        lastLoggedIndex = len(client.cs.History) // Update the last logged index
    }
}

// handleResponse processa a resposta da IA, lidando com comandos e edições automáticas.
func handleResponse(response string, client *Client) {
    // Trata comandos <RUN>
    commands := extractCommands(response)
    for _, command := range commands {
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
    }

    // Trata leituras <READ> (apenas para referência, não implementado no chatlog diretamente)
    readRequests := extractReadRequests(response)
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

    // Trata edições <EDIT> automaticamente
    editRequests := extractEditRequests(response)
    for _, req := range editRequests {
        if err := os.WriteFile(req.Filename, []byte(req.Content), 0644); err != nil {
            fmt.Printf("Erro ao editar %s: %v\n", req.Filename, err)
            client.AddMessage("user", fmt.Sprintf("Erro ao editar %s: %v", req.Filename, err))
        } else {
            fmt.Printf("Arquivo %s editado com sucesso.\n", req.Filename)
            client.AddMessage("user", fmt.Sprintf("Arquivo %s editado:\n%s", req.Filename, req.Content))
        }
    }
}

// extractCommands extrai comandos bash entre <RUN> e </RUN>.
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

// extractReadRequests extrai pedidos de leitura entre <READ> e </READ>.
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

// EditRequest representa um pedido de edição com nome do arquivo e conteúdo.
type EditRequest struct {
    Filename string
    Content  string
}

// extractEditRequests extrai pedidos de edição entre <EDIT> e </EDIT>.
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

// loadAPIKey lê a chave API do arquivo de configuração JSON.
func loadAPIKey(configFile string) string {
    data, err := os.ReadFile(configFile)
    if err != nil {
        return ""
    }
    var config struct {
        GeminiAPIKey string `json:"gemini_api_key"`
    }
    if err := json.Unmarshal(data, &config); err != nil {
        return ""
    }
    return config.GeminiAPIKey
}

// saveAPIKey escreve a chave API no arquivo de configuração JSON.
func saveAPIKey(configFile, apiKey string) {
    config := struct {
        GeminiAPIKey string `json:"gemini_api_key"`
    }{
        GeminiAPIKey: apiKey,
    }
    data, err := json.MarshalIndent(config, "", "  ")
    if err != nil {
        fmt.Printf("Error encoding config: %v\n", err)
        return
    }
    if err := os.MkdirAll(filepath.Dir(configFile), 0700); err != nil {
        fmt.Printf("Error creating config directory: %v\n", err)
        return
    }
    if err := os.WriteFile(configFile, data, 0600); err != nil {
        fmt.Printf("Error saving config: %v\n", err)
    }
}