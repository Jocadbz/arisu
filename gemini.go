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

// Client represents a client for interacting with the Gemini API.
type Client struct {
    cs *genai.ChatSession
}

// NewClient initializes a new Gemini client with the provided API key.
func NewClient(apiKey, modelName string) *Client {
    ctx := context.Background()
    genaiClient, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
    if err != nil {
        panic(err)
    }
    model := genaiClient.GenerativeModel(modelName)

    initialPrompt := fmt.Sprintf(
        "This conversation is running inside a terminal session on %s.\n\n"+
            "You are an AI assistant designed to help refactor and interact with code files, similar to ChatSH.\n\n"+
            "1. To run bash commands (e.g., 'ls', 'cat') on my computer, include them like this:\n\n"+
            "<RUN>\nshell_command_here\n</RUN>\n\n"+
            "For example:\n"+
            "<RUN>\nls && echo \"---\" && cat kind-lang.cabal\n</RUN>\n\n"+
            "2. If I ask you to read a file or you need its contents, include the filename like this:\n\n"+
            "<READ>filename.txt</READ>\n\n"+
            "I’ll send you the file content afterward.\n\n"+
            "3. If I ask you to update or refactor a file, provide the filename and the FULL updated content like this:\n\n"+
            "<EDIT>\nfilename.txt\ncomplete_new_content_here\n</EDIT>\n\n"+
            "Edits will be applied automatically with a single prompt, so ensure the content is correct, complete, and ready to overwrite the existing file.\n\n"+
            "Important:\n"+
            "- NEVER run/read/edit UNLESS I ASK FOR IT (indirectly or directly).\n"+
            "- NEVER use the tags unless you are sure that it is a valid command. If it is a placebo command, do not use the tags; the program will always pick it up.\n"+
            "- When presenting code in your responses, do NOT use triple backticks (```). Write the code as plain text directly in the response.\n"+
            "- Keep your answers concise, relevant, and focused on simplicity. Use the tags above to trigger actions when appropriate.\n"+
            "- When overwriting files, always provide the complete new version of the file, never partial changes or placeholders.",
        runtime.GOOS,
    )
    model.SystemInstruction = genai.NewUserContent(genai.Text(initialPrompt))
    cs := model.StartChat()

    return &Client{cs: cs}
}

// SendMessage sends a message to the Gemini API and streams the response.
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
                        fmt.Print(string(text))
                        fullResponse.WriteString(string(text))
                    }
                }
            }
        }
    }
    return fullResponse.String(), nil
}

// AddMessage adds a message to the conversation history.
func (c *Client) AddMessage(role, content string) {
    var genaiRole string
    if role == "user" {
        genaiRole = "user"
    } else if role == "assistant" {
        genaiRole = "model"
    } else {
        panic("invalid role")
    }
    c.cs.History = append(c.cs.History, &genai.Content{
        Parts: []genai.Part{genai.Text(content)},
        Role:  genaiRole,
    })
}

// GetHistory returns the conversation history as a slice of Messages.
func (c *Client) GetHistory() []Message {
    var history []Message
    for _, msg := range c.cs.History {
        role := msg.Role
        content := ""
        for _, part := range msg.Parts {
            if text, ok := part.(genai.Text); ok {
                content += string(text)
            }
        }
        history = append(history, Message{Role: role, Content: content})
    }
    return history
}
