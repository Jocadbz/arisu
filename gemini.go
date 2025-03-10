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

// Client representa um cliente para interagir com a API de IA generativa.
type Client struct {
    cs *genai.ChatSession
}

// NewClient inicializa um novo Client com a chave API fornecida.
func NewClient(apiKey string) *Client {
    ctx := context.Background()
    genaiClient, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
    if err != nil {
        panic(err)
    }
    model := genaiClient.GenerativeModel("gemini-1.5-flash")

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
            "Important: When presenting code in your responses, do NOT use triple backticks (```). Write the code as plain text directly in the response.\n\n"+
            "Keep your answers concise, relevant, and focused on simplicity. Use the tags above to trigger actions when appropriate.\n\n"+
            "When overwriting files, always provide the complete new version of the file, never partial changes or placeholders.",
        runtime.GOOS,
    )
    model.SystemInstruction = genai.NewUserContent(genai.Text(initialPrompt))
    cs := model.StartChat()

    return &Client{cs: cs}
}

// SendMessage envia uma mensagem para a API e transmite a resposta em tempo real.
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

// AddMessage adiciona uma mensagem ao histórico da conversa.
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