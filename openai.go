package main

import (
	"context"
	"fmt"
	"io"
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
	history := []Message{{Role: "system", Content: defaultSystemPrompt()}}
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
