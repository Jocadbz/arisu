package main

import (
    "bytes"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "runtime"
)

// Message represents a single message in the conversation.
type Message struct {
    Role    string `json:"role"`
    Content string `json:"content"`
}

// Part represents a part of the content in the API request/response.
type Part struct {
    Text string `json:"text"`
}

// Content represents a message with a role and parts.
type Content struct {
    Role  string `json:"role"`
    Parts []Part `json:"parts"`
}

// ChatRequest is the structure for sending messages to the API.
type ChatRequest struct {
    Contents []Content `json:"contents"`
}

// ChatResponse is the structure for parsing API responses.
type ChatResponse struct {
    Candidates []struct {
        Content Content `json:"content"`
    } `json:"candidates"`
    Error struct {
        Message string `json:"message"`
    } `json:"error,omitempty"`
}

// Client manages the Gemini API interaction and conversation state.
type Client struct {
    apiKey   string
    messages []Message
}

// NewClient initializes the client with an API key and an initial prompt.
func NewClient(apiKey string) *Client {
    initialPrompt := fmt.Sprintf(
        "This conversation is running inside a terminal session on %s.\n\n"+
            "To better assist me, I'll let you run bash commands on my computer.\n\n"+
            "To do so, include, anywhere in your answer, a bash script, as follows:\n\n"+
            "<RUN>\nshell_script_here\n</RUN>\n\n"+
            "For example, to create a new file, you can write:\n\n"+
            "<RUN>\ncat > hello.ts << EOL\nconsole.log(\"Hello, world!\")\nEOL\n</RUN>\n\n"+
            "And to run it, you can write:\n\n"+
            "<RUN>\nbun hello.ts\n</RUN>\n\n"+
            "I will show you the outputs of every command you run.\n\n"+
            "Keep your answers brief and to the point. Don't include unsolicited details.",
        runtime.GOOS,
    )
    return &Client{
        apiKey:   apiKey,
        messages: []Message{{Role: "user", Content: initialPrompt}},
    }
}

// SendMessage sends a user input to the Gemini API and returns the response.
func (c *Client) SendMessage(input string) (string, error) {
    // Add user message to context
    c.messages = append(c.messages, Message{Role: "user", Content: input})

    // Prepare request payload with roles
    var contents []Content
    for _, msg := range c.messages {
        role := msg.Role
        if role == "assistant" {
            role = "model" // Gemini uses "model" for assistant
        }
        contents = append(contents, Content{
            Role:  role,
            Parts: []Part{{Text: msg.Content}},
        })
    }

    reqData := ChatRequest{Contents: contents}
    data, err := json.Marshal(reqData)
    if err != nil {
        return "", fmt.Errorf("error encoding payload: %v", err)
    }

    // Build and send HTTP request
    url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent?key=%s", c.apiKey)
    req, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
    if err != nil {
        return "", fmt.Errorf("error creating request: %v", err)
    }
    req.Header.Set("Content-Type", "application/json")

    client := &http.Client{}
    resp, err := client.Do(req)
    if err != nil {
        return "", fmt.Errorf("error in request: %v", err)
    }
    defer resp.Body.Close()

    // Read raw response body
    body, err := io.ReadAll(resp.Body)
    if err != nil {
        return "", fmt.Errorf("error reading response: %v", err)
    }

    // Check status
    if resp.StatusCode != http.StatusOK {
        var errResp ChatResponse
        if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error.Message != "" {
            return "", fmt.Errorf("error %d: %s", resp.StatusCode, errResp.Error.Message)
        }
        return "", fmt.Errorf("error: API returned %d - %s", resp.StatusCode, string(body))
    }

    // Parse response
    var respData ChatResponse
    if err := json.Unmarshal(body, &respData); err != nil {
        return "", fmt.Errorf("error decoding response: %v - Raw response: %s", err, string(body))
    }

    if len(respData.Candidates) > 0 && len(respData.Candidates[0].Content.Parts) > 0 {
        assistantMsg := respData.Candidates[0].Content.Parts[0].Text
        c.messages = append(c.messages, Message{Role: "assistant", Content: assistantMsg})
        return assistantMsg, nil
    }
    return "", fmt.Errorf("error: No response from Gemini - Raw response: %s", string(body))
}

// AddMessage adds a message to the conversation context.
func (c *Client) AddMessage(role, content string) {
    c.messages = append(c.messages, Message{Role: role, Content: content})
}