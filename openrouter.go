package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// OpenRouterClient represents a client for interacting with the OpenRouter API.
type OpenRouterClient struct {
	apiKey     string
	model      string
	history    []Message
	maxHistory int
}

// NewOpenRouterClient initializes a new OpenRouter client.
func NewOpenRouterClient(apiKey, model string, maxHistory int) *OpenRouterClient {
	// Remove the "openrouter-" prefix for the API call.
	apiModel := strings.TrimPrefix(model, "openrouter-")
	history := []Message{{Role: "system", Content: defaultSystemPrompt()}}
	return &OpenRouterClient{apiKey: apiKey, model: apiModel, history: history, maxHistory: maxHistory}
}

// SendMessage sends a message to the OpenRouter API and streams the response.
func (c *OpenRouterClient) SendMessage(input string) (string, error) {
	// Truncate history if needed, keeping system prompt
	if len(c.history) > c.maxHistory {
		c.history = append([]Message{c.history[0]}, c.history[len(c.history)-(c.maxHistory-1):]...)
	}

	c.history = append(c.history, Message{Role: "user", Content: input})

	messages := make([]map[string]string, len(c.history))
	for i, msg := range c.history {
		messages[i] = map[string]string{"role": msg.Role, "content": msg.Content}
	}
	payload := map[string]interface{}{
		"messages": messages,
		"model":    c.model,
		"stream":   true,
	}
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", "https://openrouter.ai/api/v1/chat/completions", bytes.NewBuffer(jsonPayload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	reader := bufio.NewReader(resp.Body)
	var fullResponse strings.Builder
	for {
		line, err := reader.ReadString('\n')
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data: ") {
			data := line[6:]
			if data == "[DONE]" {
				break
			}
			var chunk map[string]interface{}
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}
			if choices, ok := chunk["choices"].([]interface{}); ok && len(choices) > 0 {
				if choice, ok := choices[0].(map[string]interface{}); ok {
					if delta, ok := choice["delta"].(map[string]interface{}); ok {
						if content, ok := delta["content"].(string); ok {
							fmt.Print(content)
							fullResponse.WriteString(content)
						}
					}
				}
			}
		}
	}

	fmt.Print("\n")
	responseText := fullResponse.String() + "\n"
	c.history = append(c.history, Message{Role: "assistant", Content: responseText})
	return responseText, nil
}

// AddMessage adds a message to the conversation history.
func (c *OpenRouterClient) AddMessage(role, content string) {
	c.history = append(c.history, Message{Role: role, Content: content})
}

// GetHistory returns the conversation history.
func (c *OpenRouterClient) GetHistory() []Message {
	return c.history
}
