// Command simulator creates multiple simulated chat users that read existing
// conversations and post contextual responses using an LLM.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/aeolun/superchat/pkg/botlib"
)

// Persona defines a simulated user's personality
type Persona struct {
	Name        string
	Style       string // Description of how they communicate
	Interests   string // Topics they're interested in
	ReplyChance float64 // Probability of replying to a message (0.0-1.0)
}

var defaultPersonas = []Persona{
	{
		Name:        "TechEnthusiast",
		Style:       "enthusiastic about technology, uses casual language, sometimes adds emoji",
		Interests:   "programming, gadgets, AI, open source",
		ReplyChance: 0.3,
	},
	{
		Name:        "CuriousNewbie",
		Style:       "asks lots of questions, friendly and eager to learn",
		Interests:   "learning new things, understanding how stuff works",
		ReplyChance: 0.4,
	},
	{
		Name:        "HelpfulExpert",
		Style:       "gives detailed explanations, patient, provides examples",
		Interests:   "helping others, sharing knowledge, best practices",
		ReplyChance: 0.25,
	},
	{
		Name:        "CasualChatter",
		Style:       "relaxed, makes jokes, keeps things light",
		Interests:   "general chat, humor, random topics",
		ReplyChance: 0.35,
	},
}

// LLMClient interface for different backends
type LLMClient interface {
	Generate(systemPrompt, userPrompt string) (string, error)
}

// =============================================================================
// Ollama Backend
// =============================================================================

type OllamaClient struct {
	baseURL    string
	model      string
	httpClient *http.Client
}

func NewOllamaClient(baseURL, model string) *OllamaClient {
	return &OllamaClient{
		baseURL:    baseURL,
		model:      model,
		httpClient: &http.Client{Timeout: 120 * time.Second},
	}
}

type ollamaRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
}

type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaResponse struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
	Error string `json:"error,omitempty"`
}

func (o *OllamaClient) Generate(systemPrompt, userPrompt string) (string, error) {
	messages := []ollamaMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	reqBody := ollamaRequest{
		Model:    o.model,
		Messages: messages,
		Stream:   false,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", o.baseURL+"/api/chat", bytes.NewReader(jsonBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var ollamaResp ollamaResponse
	if err := json.Unmarshal(body, &ollamaResp); err != nil {
		return "", err
	}

	if ollamaResp.Error != "" {
		return "", fmt.Errorf("ollama error: %s", ollamaResp.Error)
	}

	return strings.TrimSpace(ollamaResp.Message.Content), nil
}

// =============================================================================
// Claude Backend
// =============================================================================

type ClaudeClient struct {
	apiKey     string
	model      string
	maxTokens  int
	httpClient *http.Client
}

func NewClaudeClient(apiKey, model string, maxTokens int) *ClaudeClient {
	return &ClaudeClient{
		apiKey:     apiKey,
		model:      model,
		maxTokens:  maxTokens,
		httpClient: &http.Client{Timeout: 120 * time.Second},
	}
}

type claudeRequest struct {
	Model     string          `json:"model"`
	MaxTokens int             `json:"max_tokens"`
	System    string          `json:"system,omitempty"`
	Messages  []claudeMessage `json:"messages"`
}

type claudeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
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

func (c *ClaudeClient) Generate(systemPrompt, userPrompt string) (string, error) {
	reqBody := claudeRequest{
		Model:     c.model,
		MaxTokens: c.maxTokens,
		System:    systemPrompt,
		Messages: []claudeMessage{
			{Role: "user", Content: userPrompt},
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(jsonBody))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var claudeResp claudeResponse
	if err := json.Unmarshal(body, &claudeResp); err != nil {
		return "", err
	}

	if claudeResp.Error != nil {
		return "", fmt.Errorf("claude error: %s", claudeResp.Error.Message)
	}

	if len(claudeResp.Content) == 0 {
		return "", fmt.Errorf("empty response from Claude")
	}

	return strings.TrimSpace(claudeResp.Content[0].Text), nil
}

// SimulatedUser represents a single simulated chat participant
type SimulatedUser struct {
	bot     *botlib.Bot
	persona Persona
	llm     LLMClient
	logger  *log.Logger

	// Track messages we've already seen/responded to
	seenMessages map[uint64]bool
	seenMu       sync.Mutex
}

func NewSimulatedUser(server string, persona Persona, llm LLMClient) *SimulatedUser {
	logger := log.New(os.Stdout, fmt.Sprintf("[%s] ", persona.Name), log.LstdFlags)

	bot := botlib.New(botlib.Config{
		Server:   server,
		Nickname: persona.Name,
		Channels: []string{}, // Don't auto-join, we'll pick randomly each tick
		Logger:   logger,
	})

	return &SimulatedUser{
		bot:          bot,
		persona:      persona,
		llm:          llm,
		logger:       logger,
		seenMessages: make(map[uint64]bool),
	}
}

func (u *SimulatedUser) Run(stopCh <-chan struct{}, wg *sync.WaitGroup) {
	defer wg.Done()

	// Start bot in background
	botDone := make(chan error, 1)
	go func() {
		botDone <- u.bot.Run()
	}()

	// Give bot time to connect
	time.Sleep(2 * time.Second)

	// Simulation loop
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-stopCh:
			u.bot.Stop()
			return
		case err := <-botDone:
			if err != nil {
				u.logger.Printf("Bot error: %v", err)
			}
			return
		case <-ticker.C:
			u.maybeRespond()
		}
	}
}

func (u *SimulatedUser) maybeRespond() {
	// Fetch available channels and pick a random one
	channels, err := u.bot.FetchChannels()
	if err != nil {
		u.logger.Printf("Failed to fetch channels: %v", err)
		return
	}

	if len(channels) == 0 {
		u.logger.Printf("No channels available")
		return
	}

	channel := channels[rand.Intn(len(channels))]

	// Fetch recent threads from the selected channel
	threads, err := u.bot.FetchThreads(channel.Name, nil, 20)
	if err != nil {
		u.logger.Printf("Failed to fetch threads from #%s: %v", channel.Name, err)
		return
	}

	if len(threads) == 0 {
		// No threads exist, maybe start one
		if rand.Float64() < 0.3 { // 30% chance to start a new topic
			u.startNewThread(channel.Name)
		}
		return
	}

	// Filter to recent threads only (last 7 days) to avoid necro-posting
	maxAge := 7 * 24 * time.Hour
	cutoff := time.Now().Add(-maxAge)
	var recentThreads []botlib.Message
	for _, t := range threads {
		if t.CreatedAt.After(cutoff) {
			recentThreads = append(recentThreads, t)
		}
	}

	if len(recentThreads) == 0 {
		// No recent threads, maybe start one
		if rand.Float64() < 0.3 {
			u.startNewThread(channel.Name)
		}
		return
	}

	// Small chance to start a new thread even if threads exist
	if rand.Float64() < 0.03 {
		u.startNewThread(channel.Name)
		return
	}

	// Find threads we haven't seen
	var newThreads []botlib.Message
	u.seenMu.Lock()
	for _, t := range recentThreads {
		if !u.seenMessages[t.ID] {
			newThreads = append(newThreads, t)
			u.seenMessages[t.ID] = true
		}
	}
	u.seenMu.Unlock()

	if len(newThreads) == 0 {
		// No new threads, maybe reply to an existing one
		if rand.Float64() < 0.1 { // 10% chance to revisit
			thread := recentThreads[rand.Intn(len(recentThreads))]
			u.considerReplyToThread(thread)
		}
		return
	}

	// Consider responding to new threads
	for _, thread := range newThreads {
		// Skip our own messages
		if thread.AuthorNickname == u.persona.Name {
			continue
		}

		// Random chance based on persona
		if rand.Float64() > u.persona.ReplyChance {
			continue
		}

		u.respondToThread(thread)

		// Only respond to one thread per cycle to seem natural
		break
	}
}

func (u *SimulatedUser) considerReplyToThread(thread botlib.Message) {
	// Skip our own threads
	if thread.AuthorNickname == u.persona.Name {
		return
	}

	// Fetch replies to see the conversation
	replies, err := u.bot.FetchReplies(thread.ChannelID, thread.ID, 10)
	if err != nil {
		return
	}

	// Don't pile on if there are already many replies
	if len(replies) > 5 {
		return
	}

	// Check if we already replied
	for _, r := range replies {
		if r.AuthorNickname == u.persona.Name {
			return // Already participated
		}
	}

	// Small chance to join the conversation
	if rand.Float64() < 0.2 {
		u.respondToThread(thread)
	}
}

func (u *SimulatedUser) respondToThread(thread botlib.Message) {
	// Build context from thread and replies
	replies, _ := u.bot.FetchReplies(thread.ChannelID, thread.ID, 10)

	var conversation strings.Builder
	conversation.WriteString(fmt.Sprintf("%s: %s\n", thread.AuthorNickname, thread.Content))
	for _, r := range replies {
		conversation.WriteString(fmt.Sprintf("%s: %s\n", r.AuthorNickname, r.Content))
	}

	// Generate response using LLM
	systemPrompt := fmt.Sprintf(`You are %s, a chat user in an online forum.
Your communication style: %s
Your interests: %s

Generate a short, natural response (1-3 sentences) to the conversation below.
Don't use markdown. Be conversational and stay in character.
If the topic doesn't interest you, you can make a brief tangential comment or ask a question.`,
		u.persona.Name, u.persona.Style, u.persona.Interests)

	userPrompt := fmt.Sprintf("Here's the conversation:\n\n%s\n\nYour response as %s:",
		conversation.String(), u.persona.Name)

	response, err := u.llm.Generate(systemPrompt, userPrompt)
	if err != nil {
		u.logger.Printf("LLM error: %v", err)
		return
	}

	// Clean up response
	response = strings.TrimPrefix(response, u.persona.Name+":")
	response = strings.TrimSpace(response)

	if response == "" {
		return
	}

	// Post the reply
	_, err = u.bot.PostReply(thread.ChannelID, thread.ID, response)
	if err != nil {
		u.logger.Printf("Failed to post reply: %v", err)
		return
	}

	u.logger.Printf("Replied to thread by %s: %s", thread.AuthorNickname, truncate(response, 50))
}

func (u *SimulatedUser) startNewThread(channel string) {
	// Generate a new topic based on persona's interests and channel
	systemPrompt := fmt.Sprintf(`You are %s, a chat user in an online forum.
Your communication style: %s
Your interests: %s

You are posting in the #%s channel. Generate a new discussion topic relevant to this channel. The format should be:
TITLE

DESCRIPTION

Where TITLE is a short, engaging title (3-8 words) and DESCRIPTION is 1-3 sentences explaining the topic or asking a question. Don't use markdown formatting. Make sure the topic is appropriate for the #%s channel.`,
		u.persona.Name, u.persona.Style, u.persona.Interests, channel, channel)

	userPrompt := fmt.Sprintf("Generate a new discussion topic for the #%s channel that matches your interests:", channel)

	response, err := u.llm.Generate(systemPrompt, userPrompt)
	if err != nil {
		u.logger.Printf("LLM error generating topic: %v", err)
		return
	}

	response = strings.TrimSpace(response)
	if response == "" {
		return
	}

	// Post the new thread
	_, err = u.bot.Post(channel, nil, response)
	if err != nil {
		u.logger.Printf("Failed to post new thread in #%s: %v", channel, err)
		return
	}

	// Extract title for logging (first line)
	title := response
	if idx := strings.Index(response, "\n"); idx > 0 {
		title = response[:idx]
	}
	u.logger.Printf("Started new thread in #%s: %s", channel, truncate(title, 50))
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvIntOrDefault(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

func main() {
	// Flags take precedence over env vars
	server := flag.String("server", getEnvOrDefault("SIMULATOR_SERVER", "localhost:6465"), "Server address (env: SIMULATOR_SERVER)")
	backend := flag.String("backend", getEnvOrDefault("SIMULATOR_BACKEND", "ollama"), "LLM backend: 'ollama' or 'claude' (env: SIMULATOR_BACKEND)")
	ollamaURL := flag.String("ollama-url", getEnvOrDefault("SIMULATOR_OLLAMA_URL", "http://localhost:11434"), "Ollama server URL (env: SIMULATOR_OLLAMA_URL)")
	model := flag.String("model", getEnvOrDefault("SIMULATOR_MODEL", ""), "Model to use (env: SIMULATOR_MODEL)")
	maxTokens := flag.Int("max-tokens", getEnvIntOrDefault("SIMULATOR_MAX_TOKENS", 300), "Max tokens for response, Claude only (env: SIMULATOR_MAX_TOKENS)")
	numUsers := flag.Int("users", getEnvIntOrDefault("SIMULATOR_USERS", 3), "Number of simulated users, max 4 (env: SIMULATOR_USERS)")
	flag.Parse()

	if *numUsers > len(defaultPersonas) {
		*numUsers = len(defaultPersonas)
	}
	if *numUsers < 1 {
		*numUsers = 1
	}

	// Create LLM client based on backend
	var llm LLMClient
	switch *backend {
	case "claude":
		apiKey := os.Getenv("ANTHROPIC_API_KEY")
		if apiKey == "" {
			log.Fatal("ANTHROPIC_API_KEY environment variable required for Claude backend")
		}
		modelName := *model
		if modelName == "" {
			modelName = "claude-sonnet-4-20250514"
		}
		llm = NewClaudeClient(apiKey, modelName, *maxTokens)
		log.Printf("Starting chat simulator")
		log.Printf("  Server: %s", *server)
		log.Printf("  Backend: Claude (%s)", modelName)
		log.Printf("  Users: %d", *numUsers)
	case "ollama":
		modelName := *model
		if modelName == "" {
			modelName = "gemma3:4b"
		}
		llm = NewOllamaClient(*ollamaURL, modelName)
		log.Printf("Starting chat simulator")
		log.Printf("  Server: %s", *server)
		log.Printf("  Backend: Ollama (%s)", modelName)
		log.Printf("  Ollama URL: %s", *ollamaURL)
		log.Printf("  Users: %d", *numUsers)
	default:
		log.Fatalf("Unknown backend: %s (use 'ollama' or 'claude')", *backend)
	}

	// Create simulated users
	var users []*SimulatedUser
	for i := 0; i < *numUsers; i++ {
		persona := defaultPersonas[i]
		user := NewSimulatedUser(*server, persona, llm)
		users = append(users, user)
		log.Printf("  Created user: %s", persona.Name)
	}

	// Start all users
	stopCh := make(chan struct{})
	var wg sync.WaitGroup

	for _, user := range users {
		wg.Add(1)
		// Stagger starts to avoid connection storm
		time.Sleep(500 * time.Millisecond)
		go user.Run(stopCh, &wg)
	}

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Printf("Shutting down...")
	close(stopCh)
	wg.Wait()
	log.Printf("Done")
}
