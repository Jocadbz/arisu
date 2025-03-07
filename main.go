package main

import (
    "bufio"
    "bytes"
    "encoding/json"
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "strings"
    "io"
)

// main is the entry point of the program.
func main() {
    // Define config file path
    configDir := filepath.Join(os.Getenv("HOME"), ".config", "arisu")
    configFile := filepath.Join(configDir, "config.json")

    // Load API key from JSON config
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
        // Save the entered key to JSON config
        saveAPIKey(configFile, apiKey)
    }

    // Initialize Gemini client
    client := NewClient(apiKey)

    // Handle command-line arguments
    args := os.Args[1:]
    if len(args) > 0 {
        // Single prompt mode
        prompt := strings.Join(args, " ")
        response, err := client.SendMessage(prompt)
        if err != nil {
            fmt.Printf("Error: %v\n", err)
            return
        }
        fmt.Println(response)
        handleCommands(response, client)
        return
    }

    // Interactive chat mode
    fmt.Println("For multi-line input, end with a blank line.")
    scanner := bufio.NewScanner(os.Stdin)
    for {
        fmt.Print("$ ")
        var inputLines []string

        // Collect lines until a blank line is entered
        for {
            if !scanner.Scan() {
                return // Exit on EOF or error
            }
            line := scanner.Text()

            // Check for exit condition
            if line == "exit" {
                fmt.Println("Goodbye!")
                return
            }

            // Blank line signals the end of input
            if line == "" {
                break
            }

            // Add the line to the buffer
            inputLines = append(inputLines, line)
        }

        // Skip if no input was provided
        if len(inputLines) == 0 {
            continue
        }

        // Combine all lines into a single string with newlines
        input := strings.Join(inputLines, "\n")

        // Send the input to the API
        response, err := client.SendMessage(input)
        if err != nil {
            fmt.Printf("Error: %v\n", err)
            continue
        }

        fmt.Println(response)
        handleCommands(response, client)
    }
}

// handleCommands detects and executes bash commands in <RUN> tags.
// handleCommands detects and executes bash commands in <RUN> tags.
func handleCommands(response string, client *Client) {
    commands := extractCommands(response)
    for _, command := range commands {
        fmt.Printf("Detected command: %s\n", command)
        fmt.Print("Execute? (y/n): ")

        reader := bufio.NewReader(os.Stdin)
        confirm, _ := reader.ReadString('\n')
        confirm = strings.TrimSpace(confirm)

        if confirm == "y" {
            // Buffer to capture the command output
            var outputBuf bytes.Buffer
            // Set up the command
            cmd := exec.Command("bash", "-c", command)
            // Stream stdout to both terminal and buffer
            cmd.Stdout = io.MultiWriter(os.Stdout, &outputBuf)
            // Stream stderr to both terminal and buffer
            cmd.Stderr = io.MultiWriter(os.Stderr, &outputBuf)
            // Run the command
            err := cmd.Run()
            if err != nil {
                fmt.Printf("Command failed with error: %v\n", err)
            }
            // Retrieve the captured output
            output := outputBuf.String()
            // Add output to conversation context
            client.AddMessage("user", "Command output: "+output)
        } else {
            fmt.Println("Command not executed.")
        }
    }
}

// extractCommands extracts bash commands between <RUN> and </RUN> tags.
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

// loadAPIKey reads the API key from the JSON config file.
func loadAPIKey(configFile string) string {
    data, err := os.ReadFile(configFile)
    if err != nil {
        return "" // File doesn’t exist or can’t be read
    }
    var config struct {
        GeminiAPIKey string `json:"gemini_api_key"`
    }
    if err := json.Unmarshal(data, &config); err != nil {
        return "" // Invalid JSON
    }
    return config.GeminiAPIKey
}

// saveAPIKey writes the API key to the JSON config file.
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