package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
)

// OpenRouterClient represents a client for interacting with the OpenRouter API.
type OpenRouterClient struct {
	apiKey  string
	model   string
	history []Message
}

// NewOpenRouterClient initializes a new OpenRouter client.
func NewOpenRouterClient(apiKey, model string) *OpenRouterClient {
	// Remove the "openrouter-" prefix for the API call.
	apiModel := strings.TrimPrefix(model, "openrouter-")

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
			"- When overwriting files, always provide the complete new version of the file, never partial changes or placeholders.\n"+
			"You may also use the Agentic Mode. The code will handle for you, but when asked, you will create a file named 'AGENTSTEPS.arisu with the following structure (USE THE EDIT TAGS). Also make sure to not use any tags inside this file. Just word instructions:\n"+
			"Instructions:\n"+
			"You are running in Agentic mode. Follow the steps exactly, one by one.\n"+
			"After each step you will receive Proceed. automatically.\n"+
			"When you completed all the tasks, send the tag <END>.\n"+
			"<Other instructions are fine. Just make sure to keep the first paragraph.>"+
			"Context:\n"+
			"<Can be code, text or anything deemed essential to craft the response. Generally, it will be pure code here>\n"+
			"Steps:"+
			"- Say Hello, I am Agentic mode.\n"+
			"- Say Step 2 completed successfully.\n"+
			"- Say <END> to finish the run.",
		runtime.GOOS,
	)
	history := []Message{{Role: "system", Content: initialPrompt}}
	return &OpenRouterClient{apiKey: apiKey, model: apiModel, history: history}
}

// SendMessage sends a message to the OpenRouter API and streams the response.
func (c *OpenRouterClient) SendMessage(input string) (string, error) {
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
