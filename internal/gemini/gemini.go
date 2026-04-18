package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"google.golang.org/genai"
)

const geminiModel = "gemini-3-flash-preview"

const systemPrompt = `You are a food tracking assistant. The user describes what they ate.

Your job:
1. Extract food items and estimate macros (calories, protein, carbs, fat, fiber in grams).
2. If a photo is provided, estimate quantities from the image — do not ask about anything visible in the photo. If quantities are genuinely impossible to determine even from a photo, ask ONE short clarifying question — nothing more.
3. Once you have enough information, return the entries.

Rules:
- meal_type must be one of: breakfast, snack, lunch, dinner, supplements
- Use "supplements" for vitamins, protein powders, and other supplements (not regular food)
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
					"meal_type":   {Type: genai.TypeString, Description: "One of: breakfast, snack, lunch, dinner, supplements"},
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

var validMealTypes = map[string]bool{
	"breakfast": true,
	"snack":     true,
	"lunch":     true,
	"dinner":      true,
	"supplements": true,
}

// Validate checks an Entry for sane values before it's written to Sheets.
// The schema enforces shape; this enforces semantics.
func (e Entry) Validate() error {
	if !validMealTypes[e.MealType] {
		return fmt.Errorf("invalid meal_type %q", e.MealType)
	}
	desc := strings.TrimSpace(e.Description)
	if desc == "" {
		return fmt.Errorf("description is empty")
	}
	if len(desc) > 500 {
		return fmt.Errorf("description too long (%d chars)", len(desc))
	}
	checks := []struct {
		name string
		val  int
		max  int
	}{
		{"calories", e.Calories, 10000},
		{"protein", e.Protein, 1000},
		{"carbs", e.Carbs, 1000},
		{"fat", e.Fat, 1000},
		{"fiber", e.Fiber, 500},
	}
	for _, c := range checks {
		if c.val < 0 {
			return fmt.Errorf("%s is negative (%d)", c.name, c.val)
		}
		if c.val > c.max {
			return fmt.Errorf("%s exceeds max (%d > %d)", c.name, c.val, c.max)
		}
	}
	return nil
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

	clientOnce sync.Once
	client     *genai.Client
	clientErr  error
}

func NewService(apiKey string) *Service {
	return &Service{apiKey: apiKey, convs: make(map[string][]*genai.Content)}
}

func (s *Service) getClient(ctx context.Context) (*genai.Client, error) {
	s.clientOnce.Do(func() {
		s.client, s.clientErr = genai.NewClient(ctx, &genai.ClientConfig{
			APIKey:  s.apiKey,
			Backend: genai.BackendGeminiAPI,
		})
	})
	if s.clientErr != nil {
		return nil, fmt.Errorf("gemini client: %w", s.clientErr)
	}
	return s.client, nil
}

func buildSystemInstruction(prompt string) *genai.Content {
	return &genai.Content{
		Role: string(genai.RoleUser),
		Parts: []*genai.Part{
			{Text: prompt},
		},
	}
}

func buildChatConfig(systemInstr string) *genai.GenerateContentConfig {
	return &genai.GenerateContentConfig{
		SystemInstruction: buildSystemInstruction(systemInstr),
		ResponseMIMEType:  "application/json",
		ResponseSchema:    responseSchema,
	}
}

func buildTextConfig(systemInstr string) *genai.GenerateContentConfig {
	return &genai.GenerateContentConfig{
		SystemInstruction: buildSystemInstruction(systemInstr),
	}
}

// Chat sends a user message (and optional image) and returns (message, entries, error).
// If entries is non-empty, the response is ready for confirmation.
// If entries is empty, message contains a clarifying question.
func (s *Service) Chat(ctx context.Context, userEmail, date, message, profileCtx string, imgs []ImageData) (string, []Entry, error) {
	client, err := s.getClient(ctx)
	if err != nil {
		return "", nil, err
	}

	systemInstr := systemPrompt
	if profileCtx != "" {
		systemInstr = profileCtx + "\n\n" + systemPrompt
	}

	key := userEmail + "|" + date
	s.mu.Lock()
	history := s.convs[key]
	s.mu.Unlock()

	chatSession, err := client.Chats.Create(ctx, geminiModel, buildChatConfig(systemInstr), history)
	if err != nil {
		return "", nil, fmt.Errorf("gemini chat: %w", err)
	}

	var parts []genai.Part
	for _, img := range imgs {
		parts = append(parts, genai.Part{
			InlineData: &genai.Blob{MIMEType: img.MIMEType, Data: img.Data},
			MediaResolution: &genai.PartMediaResolution{
				Level: genai.PartMediaResolutionLevelMediaResolutionLow,
			},
		})
	}
	if message != "" {
		parts = append(parts, genai.Part{Text: message})
	}

	resp, err := chatSession.SendMessage(ctx, parts...)
	if err != nil {
		return "", nil, fmt.Errorf("gemini send: %w", err)
	}

	jsonStr := strings.TrimSpace(resp.Text())

	var result Response
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return "", nil, fmt.Errorf("parse response: %w", err)
	}

	for i, entry := range result.Entries {
		if err := entry.Validate(); err != nil {
			return "", nil, fmt.Errorf("entry %d: %w", i, err)
		}
	}

	s.mu.Lock()
	s.convs[key] = chatSession.History(true)
	s.mu.Unlock()

	return result.Message, result.Entries, nil
}

const editSystemPrompt = `You are editing an existing meal's food entries. The user's current entries are provided below.
Apply the user's requested change and return the FULL updated entry list.

Rules:
- To remove an item, simply omit it from the response
- To add an item, include it with the same meal_type
- To modify an item, return it with updated values
- meal_type must be one of: breakfast, snack, lunch, dinner, supplements
- All numeric values are integers
- Include fiber (grams) as an estimated integer (0 if unknown/negligible)
- Keep descriptions under 500 characters

Response format:
- Set "message" to a brief summary of what changed (e.g. "Removed the toast, updated egg count to 2")
- Set "entries" to the complete updated list of food items`

// EditEntries sends existing entries + an edit instruction and returns the modified entry list.
func (s *Service) EditEntries(ctx context.Context, entries []Entry, message, profileCtx string) (string, []Entry, error) {
	client, err := s.getClient(ctx)
	if err != nil {
		return "", nil, err
	}

	systemInstr := editSystemPrompt
	if profileCtx != "" {
		systemInstr = profileCtx + "\n\n" + systemInstr
	}

	// Build the user message with current entries as context.
	var b strings.Builder
	b.WriteString("Current entries:\n")
	for _, e := range entries {
		fmt.Fprintf(&b, "- %s: %d cal, %dg protein, %dg carbs, %dg fat, %dg fiber (meal: %s)\n",
			e.Description, e.Calories, e.Protein, e.Carbs, e.Fat, e.Fiber, e.MealType)
	}
	b.WriteString("\nRequested change: " + message)

	resp, err := client.Models.GenerateContent(ctx, geminiModel, []*genai.Content{
		genai.NewContentFromText(b.String(), genai.RoleUser),
	}, buildChatConfig(systemInstr))
	if err != nil {
		return "", nil, fmt.Errorf("gemini edit: %w", err)
	}

	jsonStr := strings.TrimSpace(resp.Text())
	var result Response
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return "", nil, fmt.Errorf("parse response: %w", err)
	}
	for i, entry := range result.Entries {
		if err := entry.Validate(); err != nil {
			return "", nil, fmt.Errorf("entry %d: %w", i, err)
		}
	}
	return result.Message, result.Entries, nil
}

// ClearConversation discards in-progress conversation for a user on a given date.
func (s *Service) ClearConversation(userEmail, date string) {
	s.mu.Lock()
	delete(s.convs, userEmail+"|"+date)
	s.mu.Unlock()
}

const insightsSystemPrompt = `You are a registered dietitian reviewing a week of logged food and activity data.
Output 3-5 bullet points. Report what the data actually shows — if most things are on track, say so; if most things are off, say so. Don't manufacture balance.
Be direct and clinical. No motivational language, no encouragement, no filler. State facts and numbers.
For each bullet, reference specific foods the user actually ate. When flagging a gap, suggest a concrete swap: "Swap [food they ate] for [alternative] to get +Xg [nutrient]" or "Adding [specific food] to [meal] would cover [gap]."
Each bullet must start with the • character (not * or -). Use **bold** only for the key term at the start of each bullet (e.g. • **Protein:** ...).

Nutrition benchmarks (evidence-based RDA): protein 1.2–2.0 g/kg depending on activity and goals; ~5 servings fruits/veg per day (prioritize variety and color diversity); 25-38g fiber (most adults fall short); added sugar <25g; sodium <2,300mg; saturated fat <10% of calories; omega-3 sources 2-3x/week. Pay attention to: vegetable intake (especially cruciferous vegetables, leafy greens), whole grain vs refined grain ratio, fruit/veg color variety, excessive processed food, and overall dietary pattern quality. Flag a gap only when the log consistently shows it — don't harp on the same nutrient every day or manufacture issues.

Adapt language to the user's nutrition knowledge level if provided in their profile:
- "beginner": use plain language, explain WHY a nutrient matters (e.g. "Fiber helps digestion and keeps you full"), name specific grocery items (e.g. "a bag of frozen broccoli" not "cruciferous vegetables"), keep suggestions very actionable
- "intermediate": use standard nutrition terms, brief rationale, can reference food groups and nutrient categories
- "advanced": use precise terminology, can reference nutrient density, bioavailability, dietary patterns; skip basic explanations
If no level is specified, default to beginner.`

const dayInsightsSystemPrompt = `You are a registered dietitian reviewing one day of logged food and activity data.
First line: a single-sentence takeaway (the most important observation for this day). No bullet character on this line.
Then 2-3 bullet points with supporting detail. Reference specific foods the user ate. When flagging a gap, suggest a concrete swap or addition: "Swap [food] for [alternative] to add +Xg [nutrient]" or "Adding [food] to [meal] would help with [gap]."
Be direct and clinical. No motivational language, no encouragement, no filler. State facts and numbers.
Each bullet must start with the • character (not * or -). Use **bold** only for the key term at the start of each bullet (e.g. • **Protein:** ...).

The summary will indicate whether the day is still in progress (today) or a completed past day.
- Past day: analyze the full log as-is. Do not prescribe changes for that day.
- In-progress day: assume the user will still eat more. Never flag low totals (calories, protein, etc.) just because the day isn't finished — they know dinner is coming. Only comment on something obviously lopsided in what's been logged so far (e.g. zero vegetables across three meals, very high sodium already). Frame any suggestion forward: "consider adding X to a later meal" rather than diagnosing the day as under-target.

Nutrition benchmarks (evidence-based RDA): protein 1.2–2.0 g/kg depending on activity and goals; ~5 servings fruits/veg per day (prioritize variety and color diversity); 25-38g fiber; added sugar <25g; sodium <2,300mg; saturated fat <10% of calories. Pay attention to: vegetable intake (especially cruciferous vegetables, leafy greens), whole grain vs refined grain ratio, excessive added sugar or saturated fat, and overall dietary pattern quality. Flag a gap only when the log clearly shows it — don't harp on the same nutrient every day or nitpick isolated meals.

Adapt language to the user's nutrition knowledge level if provided in their profile:
- "beginner": use plain language, explain WHY a nutrient matters, name specific grocery items, keep suggestions very actionable
- "intermediate": use standard nutrition terms, brief rationale, can reference food groups
- "advanced": use precise terminology, can reference nutrient density and dietary patterns; skip basic explanations
If no level is specified, default to beginner.`

const mealSuggestionsSystemPrompt = `You are a registered dietitian suggesting meals based on what has already been eaten and the user's profile.
Output one suggestion per requested meal. Each suggestion is a named dish with key ingredients — specific enough to act on, but not a full recipe.
Think "Lentil soup with spinach and crusty bread" or "Chicken stir-fry with broccoli and brown rice", NOT "protein + grain + vegetable" and NOT a multi-step recipe with measurements.
For each suggestion, briefly note what nutritional gap it addresses (e.g. "adds fiber" or "covers your protein gap").
Prioritize whole foods, vegetables (especially if underrepresented in the log), and dietary pattern quality. Favor dishes that increase fruit/veg variety, fiber, or omega-3s when those are low.
Draw from a wide range of cuisines. Keep dishes realistic for a home cook.
Format each as:
**Lunch:** <Dish name> — <key ingredients and what gap it addresses> (~<cal>, <protein>g protein)
Avoid repeating dishes or core ingredients from the previous day's meals (provided in context).
Tailor to the user's dietary preferences, restrictions, and goals if known. No motivational language or filler.

Adapt language to the user's nutrition knowledge level if provided:
- "beginner": explain why the suggestion helps (e.g. "broccoli is high in fiber which helps digestion"), use familiar dish names
- "intermediate": brief rationale, standard nutrition terms
- "advanced": can reference nutrient density, micronutrient coverage; skip basic explanations
If no level is specified, default to beginner.`

const weekMealSuggestionsSystemPrompt = `You are a registered dietitian providing meal planning ideas based on a week of food and activity data.
Suggest 3-5 specific, named dishes for the upcoming week. Each should be a real dish with key ingredients — specific enough to act on, but not a full recipe with steps.
Each suggestion should address a gap or pattern you see in the data (e.g. low fiber, low vegetable intake, protein slump on weekdays, monotonous lunches, excessive saturated fat or added sugar). Explicitly state what gap each dish addresses.
Prioritize suggestions that increase vegetable variety (especially cruciferous vegetables, leafy greens if missing), fiber, and overall dietary pattern quality.
Draw from a variety of cuisines across the suggestions; don't cluster around one flavor profile.
Format each as a bullet starting with • then **Dish name** — key ingredients, what gap it addresses, and rough macros (~cal, Xg protein).
Keep them weeknight-realistic. Avoid repeating dishes or core ingredients that appeared frequently in the week's data.
Tailor to the user's dietary preferences, restrictions, and goals if known. No motivational language, no filler.

Adapt language to the user's nutrition knowledge level if provided:
- "beginner": explain why each dish helps, use familiar ingredients, keep it approachable
- "intermediate": brief rationale, standard nutrition terms
- "advanced": can reference nutrient density, micronutrient coverage; skip basic explanations
If no level is specified, default to beginner.`

func (s *Service) insights(ctx context.Context, summary, profileCtx, systemPrompt string) (string, error) {
	client, err := s.getClient(ctx)
	if err != nil {
		return "", err
	}
	systemInstr := systemPrompt
	if profileCtx != "" {
		systemInstr = profileCtx + "\n\n" + systemPrompt
	}

	resp, err := client.Models.GenerateContent(ctx, geminiModel, []*genai.Content{
		genai.NewContentFromText(summary, genai.RoleUser),
	}, buildTextConfig(systemInstr))
	if err != nil {
		return "", fmt.Errorf("gemini generate: %w", err)
	}
	return strings.TrimSpace(resp.Text()), nil
}

// InsightsStream streams insight text chunks via onChunk. Returns the full text when done.
func (s *Service) InsightsStream(ctx context.Context, summary, profileCtx, sysPrompt string, onChunk func(string)) (string, error) {
	client, err := s.getClient(ctx)
	if err != nil {
		return "", err
	}
	systemInstr := sysPrompt
	if profileCtx != "" {
		systemInstr = profileCtx + "\n\n" + sysPrompt
	}

	var full strings.Builder
	for resp, err := range client.Models.GenerateContentStream(ctx, geminiModel, []*genai.Content{
		genai.NewContentFromText(summary, genai.RoleUser),
	}, buildTextConfig(systemInstr)) {
		if err != nil {
			return full.String(), fmt.Errorf("gemini stream: %w", err)
		}
		chunk := resp.Text()
		if chunk != "" {
			full.WriteString(chunk)
			if onChunk != nil {
				onChunk(chunk)
			}
		}
	}
	return strings.TrimSpace(full.String()), nil
}

// Prompt accessors for use with InsightsStream.
func (s *Service) InsightsPrompt() string          { return insightsSystemPrompt }
func (s *Service) DayInsightsPrompt() string       { return dayInsightsSystemPrompt }
func (s *Service) MealSuggestionsPrompt() string   { return mealSuggestionsSystemPrompt }
func (s *Service) WeekSuggestionsPrompt() string   { return weekMealSuggestionsSystemPrompt }
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

const singleMealSuggestionSystemPrompt = `You are a registered dietitian suggesting a single meal.
Output exactly one suggestion: a named dish with key ingredients — specific enough to act on, but not a full recipe.
Think "Lentil soup with spinach and crusty bread" or "Chicken stir-fry with broccoli and brown rice".
Briefly note what nutritional gap it addresses based on what's already been eaten today or yesterday.
Include rough macros at the end: (~cal, Xg protein).
Prioritize whole foods, vegetables (especially if underrepresented), and dietary pattern quality.
Tailor to the user's dietary preferences, restrictions, and goals if known.
No motivational language, no filler, no numbering. Just the dish suggestion in one concise paragraph.

Adapt language to the user's nutrition knowledge level if provided:
- "beginner": explain why the suggestion helps, use familiar dish names
- "intermediate": brief rationale, standard nutrition terms
- "advanced": can reference nutrient density; skip basic explanations
If no level is specified, default to beginner.`

func (s *Service) SingleMealSuggestionPrompt() string { return singleMealSuggestionSystemPrompt }

// SingleMealSuggestion generates a suggestion for one specific meal.
func (s *Service) SingleMealSuggestion(ctx context.Context, summary, profileCtx string) (string, error) {
	return s.insights(ctx, summary, profileCtx, singleMealSuggestionSystemPrompt)
}
