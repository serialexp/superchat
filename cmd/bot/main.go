// Command bot is a SuperChat bot that uses an LLM to respond to messages.
// Supports both Claude (Anthropic) and Ollama backends.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/aeolun/superchat/pkg/botlib"
)

// LLMClient interface for different backends
type LLMClient interface {
	Complete(prompt string) (string, error)
	CompleteWithContext(messages []chatMessage) (string, error)
}

// chatMessage is a generic message format used by both backends
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// =============================================================================
// Ollama Backend
// =============================================================================

type ollamaRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

type ollamaResponse struct {
	Message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"message"`
	Error string `json:"error,omitempty"`
}

type OllamaClient struct {
	baseURL      string
	model        string
	httpClient   *http.Client
	systemPrompt string
}

func NewOllamaClient(baseURL, model, systemPrompt string) *OllamaClient {
	return &OllamaClient{
		baseURL:      baseURL,
		model:        model,
		httpClient:   &http.Client{},
		systemPrompt: systemPrompt,
	}
}

func (o *OllamaClient) Complete(prompt string) (string, error) {
	messages := []chatMessage{
		{Role: "user", Content: prompt},
	}
	if o.systemPrompt != "" {
		messages = append([]chatMessage{{Role: "system", Content: o.systemPrompt}}, messages...)
	}
	return o.CompleteWithContext(messages)
}

func (o *OllamaClient) CompleteWithContext(messages []chatMessage) (string, error) {
	// Prepend system prompt if not already present
	if o.systemPrompt != "" && (len(messages) == 0 || messages[0].Role != "system") {
		messages = append([]chatMessage{{Role: "system", Content: o.systemPrompt}}, messages...)
	}

	reqBody := ollamaRequest{
		Model:    o.model,
		Messages: messages,
		Stream:   false,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", o.baseURL+"/api/chat", bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	var ollamaResp ollamaResponse
	if err := json.Unmarshal(body, &ollamaResp); err != nil {
		return "", fmt.Errorf("unmarshal response: %w", err)
	}

	if ollamaResp.Error != "" {
		return "", fmt.Errorf("Ollama error: %s", ollamaResp.Error)
	}

	return ollamaResp.Message.Content, nil
}

// =============================================================================
// Claude Backend
// =============================================================================

type claudeRequest struct {
	Model     string        `json:"model"`
	MaxTokens int           `json:"max_tokens"`
	Messages  []chatMessage `json:"messages"`
	System    string        `json:"system,omitempty"`
}

type claudeResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type ClaudeClient struct {
	apiKey       string
	model        string
	maxTokens    int
	httpClient   *http.Client
	systemPrompt string
}

func NewClaudeClient(apiKey, model string, maxTokens int, systemPrompt string) *ClaudeClient {
	return &ClaudeClient{
		apiKey:       apiKey,
		model:        model,
		maxTokens:    maxTokens,
		httpClient:   &http.Client{},
		systemPrompt: systemPrompt,
	}
}

func (c *ClaudeClient) Complete(prompt string) (string, error) {
	return c.CompleteWithContext([]chatMessage{
		{Role: "user", Content: prompt},
	})
}

func (c *ClaudeClient) CompleteWithContext(messages []chatMessage) (string, error) {
	reqBody := claudeRequest{
		Model:     c.model,
		MaxTokens: c.maxTokens,
		Messages:  messages,
		System:    c.systemPrompt,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	var claudeResp claudeResponse
	if err := json.Unmarshal(body, &claudeResp); err != nil {
		return "", fmt.Errorf("unmarshal response: %w", err)
	}

	if claudeResp.Error != nil {
		return "", fmt.Errorf("API error: %s", claudeResp.Error.Message)
	}

	if len(claudeResp.Content) == 0 {
		return "", fmt.Errorf("empty response")
	}

	return claudeResp.Content[0].Text, nil
}

// =============================================================================
// Main
// =============================================================================

func main() {
	// Command-line flags
	server := flag.String("server", "localhost:6465", "Server address (host:port)")
	nickname := flag.String("nickname", "[Bot] Assistant", "Bot nickname")
	channels := flag.String("channels", "general", "Comma-separated list of channels to join")
	backend := flag.String("backend", "ollama", "LLM backend: 'ollama' or 'claude'")
	model := flag.String("model", "", "Model to use (default: llama3.2 for ollama, claude-sonnet-4-20250514 for claude)")
	ollamaURL := flag.String("ollama-url", "http://localhost:11434", "Ollama server URL")
	maxTokens := flag.Int("max-tokens", 500, "Maximum tokens in response (Claude only)")
	systemPrompt := flag.String("system", "", "System prompt (optional)")
	flag.Parse()

	// Default system prompt
	if *systemPrompt == "" {
		*systemPrompt = `You are a helpful assistant participating in a chat room.
Keep your responses concise and friendly.
You're talking to users in a terminal-based chat application called SuperChat.
Don't use markdown formatting since the chat client doesn't render it.`
	}

	// Create LLM client based on backend
	var llm LLMClient
	switch *backend {
	case "ollama":
		if *model == "" {
			*model = "llama3.2"
		}
		llm = NewOllamaClient(*ollamaURL, *model, *systemPrompt)
		log.Printf("Using Ollama backend: %s (model: %s)", *ollamaURL, *model)

	case "claude":
		apiKey := os.Getenv("ANTHROPIC_API_KEY")
		if apiKey == "" {
			log.Fatal("ANTHROPIC_API_KEY environment variable is required for Claude backend")
		}
		if *model == "" {
			*model = "claude-sonnet-4-20250514"
		}
		llm = NewClaudeClient(apiKey, *model, *maxTokens, *systemPrompt)
		log.Printf("Using Claude backend (model: %s)", *model)

	default:
		log.Fatalf("Unknown backend: %s (use 'ollama' or 'claude')", *backend)
	}

	// Parse channels
	channelList := strings.Split(*channels, ",")
	for i := range channelList {
		channelList[i] = strings.TrimSpace(channelList[i])
	}

	// Create bot
	bot := botlib.New(botlib.Config{
		Server:   *server,
		Nickname: *nickname,
		Channels: channelList,
	})

	// Handle mentions - respond when someone @mentions the bot
	bot.OnMention(func(ctx *botlib.Context, msg *botlib.Message) {
		ctx.Log("Mentioned by %s: %s", msg.AuthorNickname, msg.Content)

		// Get the query without the mention
		query := msg.MentionedContent()
		if query == "" {
			ctx.Reply("Hi! How can I help you?")
			return
		}

		// Build context from thread if this is a reply
		var response string
		var err error

		if msg.IsReply() {
			// Fetch thread context for multi-turn conversation
			threadMsgs, fetchErr := ctx.FetchThread()
			if fetchErr != nil {
				ctx.Log("Failed to fetch thread: %v", fetchErr)
				// Fall back to single-turn
				response, err = llm.Complete(query)
			} else {
				// Build conversation history
				var messages []chatMessage
				for _, m := range threadMsgs {
					role := "user"
					if m.AuthorNickname == ctx.BotNickname() {
						role = "assistant"
					}
					content := m.Content
					if m.MentionsMe() {
						content = m.MentionedContent()
					}
					messages = append(messages, chatMessage{
						Role:    role,
						Content: fmt.Sprintf("%s: %s", m.AuthorNickname, content),
					})
				}
				// Add the current message
				messages = append(messages, chatMessage{
					Role:    "user",
					Content: fmt.Sprintf("%s: %s", msg.AuthorNickname, query),
				})
				response, err = llm.CompleteWithContext(messages)
			}
		} else {
			// Single-turn for new threads
			response, err = llm.Complete(fmt.Sprintf("%s asks: %s", msg.AuthorNickname, query))
		}

		if err != nil {
			ctx.Log("LLM error: %v", err)
			ctx.Reply("Sorry, I encountered an error. Please try again.")
			return
		}

		if err := ctx.Reply(response); err != nil {
			ctx.Log("Failed to reply: %v", err)
		}
	})

	// Handle thread replies - continue conversation if someone replies to a thread we're in
	bot.OnThreadReply(func(ctx *botlib.Context, msg *botlib.Message) {
		ctx.Log("Thread reply from %s: %s", msg.AuthorNickname, msg.Content)

		// Fetch thread context
		threadMsgs, err := ctx.FetchThread()
		if err != nil {
			ctx.Log("Failed to fetch thread: %v", err)
			return
		}

		// Build conversation history
		var messages []chatMessage
		for _, m := range threadMsgs {
			role := "user"
			if m.AuthorNickname == ctx.BotNickname() {
				role = "assistant"
			}
			content := m.Content
			if m.MentionsMe() {
				content = m.MentionedContent()
			}
			messages = append(messages, chatMessage{
				Role:    role,
				Content: fmt.Sprintf("%s: %s", m.AuthorNickname, content),
			})
		}

		response, err := llm.CompleteWithContext(messages)
		if err != nil {
			ctx.Log("LLM error: %v", err)
			ctx.Reply("Sorry, I encountered an error. Please try again.")
			return
		}

		if err := ctx.Reply(response); err != nil {
			ctx.Log("Failed to reply: %v", err)
		}
	})

	// Run the bot
	log.Printf("Starting bot...")
	log.Printf("  Server: %s", *server)
	log.Printf("  Nickname: %s", *nickname)
	log.Printf("  Channels: %v", channelList)

	if err := bot.Run(); err != nil {
		log.Fatalf("Bot error: %v", err)
	}
}
