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

const geminiModel = "gemini-3-flash-preview"

const systemPrompt = `You are a food tracking assistant. The user describes what they ate.

Your job:
1. Extract food items and estimate macros (calories, protein, carbs, fat, fiber in grams).
2. If a photo is provided, estimate quantities from the image — do not ask about anything visible in the photo. If quantities are genuinely impossible to determine even from a photo, ask ONE short clarifying question — nothing more.
3. Once you have enough information, return the entries.

Rules:
- meal_type must be one of: breakfast, snack, lunch, dinner
- All numeric values are integers (round estimates are fine)
- Multiple foods in one meal → multiple entries, same meal_type
- Use reasonable common serving sizes for estimates
- Include fiber (grams) as an estimated integer (0 if unknown/negligible)

Response format:
- If you need clarification: set "message" to your question, leave "entries" as empty array
- If ready to log: set "message" to a brief confirmation like "Got it!", populate "entries" with the food items`

// responseSchema defines the JSON structure for Gemini responses.
var responseSchema = &genai.Schema{
	Type: genai.TypeObject,
	Properties: map[string]*genai.Schema{
		"message": {
			Type:        genai.TypeString,
			Description: "A brief message to the user - either a clarifying question or confirmation",
		},
		"entries": {
			Type:        genai.TypeArray,
			Description: "Food entries to log (empty if asking a clarifying question)",
			Items: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"meal_type":   {Type: genai.TypeString, Description: "One of: breakfast, snack, lunch, dinner"},
					"description": {Type: genai.TypeString, Description: "Brief description of the food"},
					"calories":    {Type: genai.TypeInteger},
					"protein":     {Type: genai.TypeInteger, Description: "Grams of protein"},
					"carbs":       {Type: genai.TypeInteger, Description: "Grams of carbohydrates"},
					"fat":         {Type: genai.TypeInteger, Description: "Grams of fat"},
					"fiber":       {Type: genai.TypeInteger, Description: "Grams of fiber"},
				},
				Required: []string{"meal_type", "description", "calories", "protein", "carbs", "fat", "fiber"},
			},
		},
	},
	Required: []string{"message", "entries"},
}

// Entry is a structured food log entry.
type Entry struct {
	MealType    string `json:"meal_type"`
	Description string `json:"description"`
	Calories    int    `json:"calories"`
	Protein     int    `json:"protein"`
	Carbs       int    `json:"carbs"`
	Fat         int    `json:"fat"`
	Fiber       int    `json:"fiber"`
}

// Response is the structured response from Gemini.
type Response struct {
	Message string  `json:"message"`
	Entries []Entry `json:"entries"`
}

// ImageData carries an inline image to include alongside a chat message.
type ImageData struct {
	MIMEType string
	Data     []byte
}

// Service manages per-user Gemini conversation history in memory.
type Service struct {
	apiKey string
	mu     sync.Mutex
	convs  map[string][]*genai.Content // keyed by userEmail|date
}

func NewService(apiKey string) *Service {
	return &Service{apiKey: apiKey, convs: make(map[string][]*genai.Content)}
}

// Chat sends a user message (and optional image) and returns (message, entries, error).
// If entries is non-empty, the response is ready for confirmation.
// If entries is empty, message contains a clarifying question.
func (s *Service) Chat(ctx context.Context, userEmail, date, message, profileCtx string, imgs []ImageData) (string, []Entry, error) {
	client, err := genai.NewClient(ctx, option.WithAPIKey(s.apiKey))
	if err != nil {
		return "", nil, fmt.Errorf("gemini client: %w", err)
	}
	defer client.Close()

	model := client.GenerativeModel(geminiModel)

	// Configure structured JSON output
	model.ResponseMIMEType = "application/json"
	model.ResponseSchema = responseSchema

	systemInstr := systemPrompt
	if profileCtx != "" {
		systemInstr = profileCtx + "\n\n" + systemPrompt
	}
	model.SystemInstruction = &genai.Content{
		Parts: []genai.Part{genai.Text(systemInstr)},
	}

	key := userEmail + "|" + date
	s.mu.Lock()
	history := s.convs[key]
	s.mu.Unlock()

	chatSession := model.StartChat()
	chatSession.History = history

	var parts []genai.Part
	for _, img := range imgs {
		parts = append(parts, genai.Blob{MIMEType: img.MIMEType, Data: img.Data})
	}
	if message != "" {
		parts = append(parts, genai.Text(message))
	}

	resp, err := chatSession.SendMessage(ctx, parts...)
	if err != nil {
		return "", nil, fmt.Errorf("gemini send: %w", err)
	}

	// Extract JSON response
	var jsonStr string
	for _, part := range resp.Candidates[0].Content.Parts {
		if txt, ok := part.(genai.Text); ok {
			jsonStr += string(txt)
		}
	}

	var result Response
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return "", nil, fmt.Errorf("parse response: %w", err)
	}

	s.mu.Lock()
	s.convs[key] = chatSession.History
	s.mu.Unlock()

	return result.Message, result.Entries, nil
}

// ClearConversation discards in-progress conversation for a user on a given date.
func (s *Service) ClearConversation(userEmail, date string) {
	s.mu.Lock()
	delete(s.convs, userEmail+"|"+date)
	s.mu.Unlock()
}

const insightsSystemPrompt = `You are a nutrition analyst reviewing a week of logged food and activity data.
Output 3-5 bullet points. Report what the data actually shows — if most things are on track, say so; if most things are off, say so. Don't manufacture balance.
Be direct and clinical. No motivational language, no encouragement, no filler. State facts and numbers.
One concrete change for next week.
Each bullet must start with the • character (not * or -). Use **bold** only for the key term at the start of each bullet (e.g. • **Protein:** ...).

Protein targets (ACSM/ISSN guidelines): use 1.2–1.6 g/kg for active adults maintaining fitness; 1.6–2.0 g/kg only if the user's goals explicitly include building muscle or strength training focus. Do not push toward the upper end of a range unless the profile justifies it.`

const dayInsightsSystemPrompt = `You are a nutrition analyst reviewing one day of logged food and activity data.
First line: a single-sentence takeaway (the most important observation for this day). No bullet character on this line.
Then 2-3 bullet points with supporting detail. Be direct and clinical. No motivational language, no encouragement, no filler. State facts and numbers.
Each bullet must start with the • character (not * or -). Use **bold** only for the key term at the start of each bullet (e.g. • **Protein:** ...).

Protein targets (ACSM/ISSN guidelines): use 1.2–1.6 g/kg for active adults maintaining fitness; 1.6–2.0 g/kg only if the user's goals explicitly include building muscle or strength training focus. Do not push toward the upper end of a range unless the profile justifies it.`

const mealSuggestionsSystemPrompt = `You are a nutrition assistant suggesting meals based on what has already been eaten and the user's profile.
Output one suggestion per requested meal. Each suggestion is a named, recipe-style dish — not a list of food groups.
Think "Thai basil chicken with jasmine rice" or "sheet-pan harissa salmon with chickpeas and lemon yogurt", NOT "protein + grain + vegetable".
Draw from a wide range of cuisines and cooking styles. Vary across suggestions — don't keep defaulting to the same proteins, grains, or flavor profiles.
Keep dishes realistic for a home cook: weeknight-feasible, ~30 minutes or less unless clearly worth it.
Format each as:
**Lunch:** <Dish name> — <one sentence describing key components and flavors> (~<cal>, <protein>g protein)
Avoid repeating dishes or core ingredients from the previous day's meals (provided in context).
Tailor to the user's dietary preferences, restrictions, and goals if known. No motivational language or filler.`

const weekMealSuggestionsSystemPrompt = `You are a nutrition assistant providing meal planning ideas based on a week of food and activity data.
Suggest 3-5 specific, named recipe-style dishes for the upcoming week. Each should be a real dish you could look up or cook — "Korean beef bulgogi bowls", "lemon-garlic shrimp pasta", "black bean and sweet potato tacos" — NOT generic "protein + grain + veg" combinations.
Each suggestion should address a gap or pattern you see in the data (e.g. low fiber, protein slump on weekdays, monotonous lunches).
Draw from a variety of cuisines and cooking styles across the suggestions; don't cluster around one flavor profile.
Format each as a bullet starting with • then **Dish name** — one sentence on key components, the gap it addresses, and rough macros.
Keep them weeknight-realistic. Avoid repeating dishes or core ingredients that appeared frequently in the week's data.
Tailor to the user's dietary preferences, restrictions, and goals if known. No motivational language, no filler.`

func (s *Service) insights(ctx context.Context, summary, profileCtx, systemPrompt string) (string, error) {
	client, err := genai.NewClient(ctx, option.WithAPIKey(s.apiKey))
	if err != nil {
		return "", fmt.Errorf("gemini client: %w", err)
	}
	defer client.Close()

	model := client.GenerativeModel(geminiModel)
	systemInstr := systemPrompt
	if profileCtx != "" {
		systemInstr = profileCtx + "\n\n" + systemPrompt
	}
	model.SystemInstruction = &genai.Content{
		Parts: []genai.Part{genai.Text(systemInstr)},
	}

	resp, err := model.GenerateContent(ctx, genai.Text(summary))
	if err != nil {
		return "", fmt.Errorf("gemini generate: %w", err)
	}
	var sb strings.Builder
	for _, part := range resp.Candidates[0].Content.Parts {
		if txt, ok := part.(genai.Text); ok {
			sb.WriteString(string(txt))
		}
	}
	return strings.TrimSpace(sb.String()), nil
}

// Insights generates a free-form weekly analysis given a text summary of the week's data.
func (s *Service) Insights(ctx context.Context, weekSummary, profileCtx string) (string, error) {
	return s.insights(ctx, weekSummary, profileCtx, insightsSystemPrompt)
}

// DayInsights generates a single-day analysis given a text summary of the day's data.
func (s *Service) DayInsights(ctx context.Context, daySummary, profileCtx string) (string, error) {
	return s.insights(ctx, daySummary, profileCtx, dayInsightsSystemPrompt)
}

// MealSuggestions generates meal suggestions given context about eaten/missing meals.
func (s *Service) MealSuggestions(ctx context.Context, summary, profileCtx string) (string, error) {
	return s.insights(ctx, summary, profileCtx, mealSuggestionsSystemPrompt)
}

// WeekMealSuggestions generates meal planning suggestions based on a week of data.
func (s *Service) WeekMealSuggestions(ctx context.Context, weekSummary, profileCtx string) (string, error) {
	return s.insights(ctx, weekSummary, profileCtx, weekMealSuggestionsSystemPrompt)
}
