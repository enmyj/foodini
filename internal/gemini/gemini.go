package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

const systemPrompt = `You are a food tracking assistant. The user describes what they ate.

Your job:
1. Extract food items and estimate macros (calories, protein, carbs, fat in grams).
2. If quantities are ambiguous, ask ONE short clarifying question — nothing more.
3. Once you have enough information, show a friendly human-readable summary:

   Here's what I'm logging:
   • [description] ([meal_type]) — [calories] cal, [protein]g P, [carbs]g C, [fat]g F

   Does this look right?

   Then include the JSON in a code block:
` + "```json" + `
   {"entries":[{"meal_type":"breakfast","description":"oatmeal with milk","calories":300,"protein":8,"carbs":54,"fat":6,"fiber":4}]}
` + "```" + `

4. If the user says yes / ok / looks good / save it / confirm, repeat the JSON code block exactly so it can be processed.

Rules:
- meal_type must be one of: breakfast, snack, lunch, dinner
- All numeric values are integers (round estimates are fine)
- Multiple foods in one meal → multiple entries, same meal_type
- Use reasonable common serving sizes for estimates
- Include fiber (grams) as an estimated integer (0 if unknown/negligible)`

// Entry is a structured food log entry extracted from a Gemini response.
type Entry struct {
	MealType    string `json:"meal_type"`
	Description string `json:"description"`
	Calories    int    `json:"calories"`
	Protein     int    `json:"protein"`
	Carbs       int    `json:"carbs"`
	Fat         int    `json:"fat"`
	Fiber       int    `json:"fiber"`
}

// ParseEntries attempts to extract a []Entry from a Gemini response string.
// Returns (entries, true) if a valid JSON entry list is found.
// Returns (nil, false) if the response is a question or clarification.
func ParseEntries(raw string) ([]Entry, bool) {
	start := strings.Index(raw, `{"entries"`)
	if start < 0 {
		return nil, false
	}
	end := strings.LastIndex(raw, "}")
	if end < start {
		return nil, false
	}
	candidate := raw[start : end+1]

	var result struct {
		Entries []Entry `json:"entries"`
	}
	if err := json.Unmarshal([]byte(candidate), &result); err != nil {
		return nil, false
	}
	if len(result.Entries) == 0 {
		return nil, false
	}
	return result.Entries, true
}

// Service manages per-user Gemini conversation history in memory.
type Service struct {
	apiKey string
	mu     sync.Mutex
	convs  map[string][]*genai.Content // keyed by userEmail
}

func NewService(apiKey string) *Service {
	return &Service{apiKey: apiKey, convs: make(map[string][]*genai.Content)}
}

// Chat sends a user message and returns (responseText, entries, error).
// If Gemini returns structured entries, history is cleared and entries are non-nil.
// If Gemini asks a clarifying question, history is preserved for the next turn.
func (s *Service) Chat(ctx context.Context, userEmail, message string) (string, []Entry, error) {
	client, err := genai.NewClient(ctx, option.WithAPIKey(s.apiKey))
	if err != nil {
		return "", nil, fmt.Errorf("gemini client: %w", err)
	}
	defer client.Close()

	model := client.GenerativeModel("gemini-2.5-flash")
	model.SystemInstruction = &genai.Content{
		Parts: []genai.Part{genai.Text(systemPrompt)},
	}

	s.mu.Lock()
	history := s.convs[userEmail]
	s.mu.Unlock()

	chatSession := model.StartChat()
	chatSession.History = history

	resp, err := chatSession.SendMessage(ctx, genai.Text(message))
	if err != nil {
		return "", nil, fmt.Errorf("gemini send: %w", err)
	}

	var sb strings.Builder
	for _, part := range resp.Candidates[0].Content.Parts {
		sb.WriteString(fmt.Sprintf("%v", part))
	}
	responseText := sb.String()

	entries, ok := ParseEntries(responseText)

	// Always persist conversation history.
	// Clearing happens when the user confirms via /api/chat/confirm.
	s.mu.Lock()
	s.convs[userEmail] = chatSession.History
	s.mu.Unlock()

	if ok {
		return responseText, entries, nil
	}
	return responseText, nil, nil
}

// ClearConversation discards in-progress conversation for a user.
func (s *Service) ClearConversation(userEmail string) {
	s.mu.Lock()
	delete(s.convs, userEmail)
	s.mu.Unlock()
}
