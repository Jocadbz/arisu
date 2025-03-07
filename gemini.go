package main

import (
    "context"
    "fmt"
    "runtime"
    "strings"

    "github.com/google/generative-ai-go/genai"
    "google.golang.org/api/iterator"
    "google.golang.org/api/option"
)

// Client represents a client for interacting with the generative AI API.
type Client struct {
    cs *genai.ChatSession // Chat session to manage conversation history
}

// NewClient initializes a new Client with the provided API key.
func NewClient(apiKey string) *Client {
    ctx := context.Background()
    genaiClient, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
    if err != nil {
        panic(err) // In production, consider returning an error instead
    }
    model := genaiClient.GenerativeModel("gemini-1.5-flash")

    // Set the initial system instruction (context for the conversation)
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
    model.SystemInstruction = genai.NewUserContent(genai.Text(initialPrompt))
    cs := model.StartChat()

    return &Client{cs: cs}
}

// SendMessage sends a message to the API and streams the response in real-time.
// It returns the full response string for further processing (e.g., command handling).
func (c *Client) SendMessage(input string) (string, error) {
    ctx := context.Background()
    iter := c.cs.SendMessageStream(ctx, genai.Text(input))
    var fullResponse strings.Builder

    for {
        resp, err := iter.Next()
        if err == iterator.Done {
            break
        }
        if err != nil {
            return "", err
        }
        for _, cand := range resp.Candidates {
            if cand.Content != nil {
                for _, part := range cand.Content.Parts {
                    if text, ok := part.(genai.Text); ok {
                        fmt.Print(string(text)) // Stream output to terminal
                        fullResponse.WriteString(string(text))
                    }
                }
            }
        }
    }
    return fullResponse.String(), nil
}

// AddMessage adds a message to the conversation history (e.g., command outputs).
func (c *Client) AddMessage(role, content string) {
    var genaiRole string
    if role == "user" {
        genaiRole = "user"
    } else if role == "assistant" {
        genaiRole = "model"
    } else {
        panic("invalid role") // In production, consider returning an error
    }
    c.cs.History = append(c.cs.History, &genai.Content{
        Parts: []genai.Part{genai.Text(content)},
        Role:  genaiRole,
    })
}