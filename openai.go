package main

import (
    "context"
    "fmt"
    "io"
    "runtime"
    "strings"

    "github.com/sashabaranov/go-openai"
)

// OpenAIClient representa um cliente para interação com a API da OpenAI.
type OpenAIClient struct {
    client  *openai.Client
    model   string
    history []Message
}

// NewOpenAIClient inicializa um novo cliente OpenAI com a chave API e o modelo fornecidos.
func NewOpenAIClient(apiKey, model string) *OpenAIClient {
    client := openai.NewClient(apiKey)
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
    history := []Message{{Role: "system", Content: initialPrompt}}
    return &OpenAIClient{client: client, model: model, history: history}
}

// SendMessage envia uma mensagem para a API da OpenAI e transmite a resposta em tempo real.
func (c *OpenAIClient) SendMessage(input string) (string, error) {
    c.history = append(c.history, Message{Role: "user", Content: input})

    messages := make([]openai.ChatCompletionMessage, len(c.history))
    for i, msg := range c.history {
        messages[i] = openai.ChatCompletionMessage{
            Role:    msg.Role,
            Content: msg.Content,
        }
    }

    req := openai.ChatCompletionRequest{
        Model:    c.model,
        Messages: messages,
        Stream:   true,
    }

    stream, err := c.client.CreateChatCompletionStream(context.Background(), req)
    if err != nil {
        return "", err
    }
    defer stream.Close()

    var fullResponse strings.Builder
    for {
        response, err := stream.Recv()
        if err == io.EOF {
            break
        }
        if err != nil {
            return "", err
        }
        if len(response.Choices) > 0 {
            content := response.Choices[0].Delta.Content
            fmt.Print(content)
            fullResponse.WriteString(content)
        }
    }

    // Adiciona uma nova linha ao final da resposta
    fmt.Print("\n")
    responseText := fullResponse.String() + "\n"
    c.history = append(c.history, Message{Role: "assistant", Content: responseText})
    return responseText, nil
}

// AddMessage adiciona uma mensagem ao histórico da conversa.
func (c *OpenAIClient) AddMessage(role, content string) {
    c.history = append(c.history, Message{Role: role, Content: content})
}

// GetHistory retorna o histórico da conversa.
func (c *OpenAIClient) GetHistory() []Message {
    return c.history
}
