package main

import (
	"context"
	"fmt"
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
	model.SystemInstruction = genai.NewUserContent(genai.Text(defaultSystemPrompt()))
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
	fmt.Print("\n")
	return fullResponse.String() + "\n", nil
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

	historyLen := len(c.cs.History)
	if historyLen > 0 {
		lastMessage := c.cs.History[historyLen-1]
		if lastMessage.Role == "user" && genaiRole == "user" {
			// Merge with previous user message
			var newParts []genai.Part
			newParts = append(newParts, lastMessage.Parts...)
			newParts = append(newParts, genai.Text("\n\n"+content))
			lastMessage.Parts = newParts
			return
		}
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
