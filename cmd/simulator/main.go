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

// OllamaClient for LLM completions
type OllamaClient struct {
	baseURL    string
	model      string
	httpClient *http.Client
}

func NewOllamaClient(baseURL, model string) *OllamaClient {
	return &OllamaClient{
		baseURL:    baseURL,
		model:      model,
		httpClient: &http.Client{Timeout: 60 * time.Second},
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

// SimulatedUser represents a single simulated chat participant
type SimulatedUser struct {
	bot     *botlib.Bot
	persona Persona
	llm     *OllamaClient
	logger  *log.Logger
	channel string

	// Track messages we've already seen/responded to
	seenMessages map[uint64]bool
	seenMu       sync.Mutex
}

func NewSimulatedUser(server, channel string, persona Persona, llm *OllamaClient) *SimulatedUser {
	logger := log.New(os.Stdout, fmt.Sprintf("[%s] ", persona.Name), log.LstdFlags)

	bot := botlib.New(botlib.Config{
		Server:   server,
		Nickname: persona.Name,
		Channels: []string{channel},
		Logger:   logger,
	})

	return &SimulatedUser{
		bot:          bot,
		persona:      persona,
		llm:          llm,
		logger:       logger,
		channel:      channel,
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
	// Fetch recent threads
	threads, err := u.bot.FetchThreads(u.channel, nil, 20)
	if err != nil {
		u.logger.Printf("Failed to fetch threads: %v", err)
		return
	}

	if len(threads) == 0 {
		return
	}

	// Find threads we haven't seen
	var newThreads []botlib.Message
	u.seenMu.Lock()
	for _, t := range threads {
		if !u.seenMessages[t.ID] {
			newThreads = append(newThreads, t)
			u.seenMessages[t.ID] = true
		}
	}
	u.seenMu.Unlock()

	if len(newThreads) == 0 {
		// No new threads, maybe reply to an existing one
		if rand.Float64() < 0.1 { // 10% chance to revisit
			thread := threads[rand.Intn(len(threads))]
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

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func main() {
	server := flag.String("server", "localhost:6465", "Server address")
	channel := flag.String("channel", "general", "Channel to simulate in")
	ollamaURL := flag.String("ollama-url", "http://localhost:11434", "Ollama server URL")
	model := flag.String("model", "gemma3:4b", "Ollama model to use")
	numUsers := flag.Int("users", 3, "Number of simulated users (max 4)")
	flag.Parse()

	if *numUsers > len(defaultPersonas) {
		*numUsers = len(defaultPersonas)
	}
	if *numUsers < 1 {
		*numUsers = 1
	}

	llm := NewOllamaClient(*ollamaURL, *model)

	log.Printf("Starting chat simulator")
	log.Printf("  Server: %s", *server)
	log.Printf("  Channel: %s", *channel)
	log.Printf("  Users: %d", *numUsers)
	log.Printf("  Model: %s", *model)

	// Create simulated users
	var users []*SimulatedUser
	for i := 0; i < *numUsers; i++ {
		persona := defaultPersonas[i]
		user := NewSimulatedUser(*server, *channel, persona, llm)
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
