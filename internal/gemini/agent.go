package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"google.golang.org/genai"
)

const agentSystemPrompt = `You are a food tracking assistant. The user is in their daily log drawer and may want to add or edit meals, log activity, log a stool, save a favorite, ask questions about their log, or just chat.

Pick the right tool based on context. If no tool fits, just reply in plain text.

Rules:
- meal_type ∈ {breakfast, snack, lunch, dinner, supplements}
- All numeric macros are integers (round estimates are fine). Fiber: 0 if unknown.
- For "edit_meal", return the FULL replacement entry list (omit removed items, include unchanged items).
- For "log_meal", default to the meal_type currently in context unless the user names a different one.
- If the user says "I ate the same thing as yesterday's <meal>" (or similar), look at "Yesterday's meals" in context and call log_meal with those exact items.
- If the user references a favorite by name, look at "Available favorites" in context and call log_meal with that favorite's macros.
- If a photo is provided, estimate quantities from the image — do not ask about anything visible. Only ask ONE clarifying question if quantities are genuinely impossible to determine.
- Don't call read_log unless the user asks a question that needs data you don't already have in context.
- Keep text replies brief and conversational. After successfully calling a tool, a short confirmation like "Got it!" or "Done — added 320 cal." is enough.`

// agentTools returns the function declarations available to the agent.
func agentTools() []*genai.Tool {
	entryItem := &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"description": {Type: genai.TypeString, Description: "Brief description of the food"},
			"calories":    {Type: genai.TypeInteger},
			"protein":     {Type: genai.TypeInteger, Description: "Grams of protein"},
			"carbs":       {Type: genai.TypeInteger, Description: "Grams of carbohydrates"},
			"fat":         {Type: genai.TypeInteger, Description: "Grams of fat"},
			"fiber":       {Type: genai.TypeInteger, Description: "Grams of fiber (0 if unknown)"},
		},
		Required: []string{"description", "calories", "protein", "carbs", "fat", "fiber"},
	}

	return []*genai.Tool{{
		FunctionDeclarations: []*genai.FunctionDeclaration{
			{
				Name:        "log_meal",
				Description: "Log one or more food items as a new meal entry. Use when the user describes something they ate.",
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"meal_type": {
							Type:        genai.TypeString,
							Description: "breakfast | snack | lunch | dinner | supplements",
						},
						"items": {
							Type:  genai.TypeArray,
							Items: entryItem,
						},
					},
					Required: []string{"meal_type", "items"},
				},
			},
			{
				Name:        "edit_meal",
				Description: "Replace the entries in the meal currently in context. Use for corrections like 'actually 2 tortillas' or 'remove the toast'. Return the FULL updated list.",
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"items": {Type: genai.TypeArray, Items: entryItem},
					},
					Required: []string{"items"},
				},
			},
			{
				Name:        "log_activity",
				Description: "Record an activity (workout, walk, exercise) for the current date.",
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"text": {Type: genai.TypeString, Description: "Description of the activity, e.g. '30min run'"},
						"append": {
							Type:        genai.TypeBoolean,
							Description: "If true, append to existing activity text; if false, replace it. Default true.",
						},
					},
					Required: []string{"text"},
				},
			},
			{
				Name:        "log_stool",
				Description: "Mark that a bowel movement occurred today, optionally with notes.",
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"notes": {Type: genai.TypeString, Description: "Optional notes"},
					},
				},
			},
			{
				Name:        "add_favorite",
				Description: "Save a meal item to the user's favorites for quick re-logging later.",
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"description": {Type: genai.TypeString},
						"meal_type":   {Type: genai.TypeString, Description: "breakfast | snack | lunch | dinner | supplements"},
						"calories":    {Type: genai.TypeInteger},
						"protein":     {Type: genai.TypeInteger},
						"carbs":       {Type: genai.TypeInteger},
						"fat":         {Type: genai.TypeInteger},
						"fiber":       {Type: genai.TypeInteger},
					},
					Required: []string{"description", "meal_type", "calories", "protein", "carbs", "fat", "fiber"},
				},
			},
			{
				Name:        "read_log",
				Description: "Read the user's food log for a date or date range. Use when the user asks a question whose answer requires log data not already in context.",
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"start_date": {Type: genai.TypeString, Description: "YYYY-MM-DD"},
						"end_date":   {Type: genai.TypeString, Description: "YYYY-MM-DD (omit for single day)"},
					},
					Required: []string{"start_date"},
				},
			},
		},
	}}
}

// AgentEntry mirrors a food item in tool args.
type AgentEntry struct {
	Description string `json:"description"`
	Calories    int    `json:"calories"`
	Protein     int    `json:"protein"`
	Carbs       int    `json:"carbs"`
	Fat         int    `json:"fat"`
	Fiber       int    `json:"fiber"`
}

// AgentToolCall represents one tool invocation the agent wants to make.
// The handler (api package) executes it and returns the result back via Continue.
type AgentToolCall struct {
	Name string
	Args map[string]any
}

// AgentToolResult is the result of executing a tool, fed back to the model.
// Output is a free-form map serialized to JSON for the model.
type AgentToolResult struct {
	Output map[string]any
}

// AgentTurn is the result of one model turn.
// If ToolCalls is non-empty, execute them and call Continue.
// Otherwise the run is done and Message is the final user-facing reply.
type AgentTurn struct {
	Message   string
	ToolCalls []AgentToolCall
}

// AgentSession holds the model conversation history for the agent.
type AgentSession struct {
	mu       sync.Mutex
	history  []*genai.Content
	systemIn string
}

// AgentContext describes the drawer state passed to the agent each turn.
type AgentContext struct {
	Date              string                 // YYYY-MM-DD currently being viewed
	SelectedMeal      string                 // breakfast/lunch/etc or ""
	CurrentEntries    []Entry                // entries in selected meal (for edit_meal)
	YesterdayByMeal   map[string][]Entry     // for "same as yesterday's lunch"
	Favorites         []string               // descriptions only — model can match by name
	Profile           string                 // pre-formatted profile context
	TodaysActivity    string                 // existing activity text for the date
	TodaysStool       bool                   // already logged today
	Extra             map[string]any         // future extensibility
}

func formatAgentContext(ac AgentContext) string {
	var b strings.Builder
	if ac.Profile != "" {
		b.WriteString(ac.Profile)
		b.WriteString("\n\n")
	}
	if ac.Date != "" {
		fmt.Fprintf(&b, "Today's date (user's local): %s\n", ac.Date)
	}
	if ac.SelectedMeal != "" {
		fmt.Fprintf(&b, "Currently in drawer: %s\n", ac.SelectedMeal)
	}
	if len(ac.CurrentEntries) > 0 {
		b.WriteString("Current entries in this meal:\n")
		for _, e := range ac.CurrentEntries {
			fmt.Fprintf(&b, "  - %s (%dcal, %dgP, %dgC, %dgF, %dgFib)\n",
				e.Description, e.Calories, e.Protein, e.Carbs, e.Fat, e.Fiber)
		}
	}
	if len(ac.YesterdayByMeal) > 0 {
		b.WriteString("Yesterday's meals:\n")
		for meal, entries := range ac.YesterdayByMeal {
			if len(entries) == 0 {
				continue
			}
			fmt.Fprintf(&b, "  %s:\n", meal)
			for _, e := range entries {
				fmt.Fprintf(&b, "    - %s (%dcal, %dgP, %dgC, %dgF, %dgFib)\n",
					e.Description, e.Calories, e.Protein, e.Carbs, e.Fat, e.Fiber)
			}
		}
	}
	if len(ac.Favorites) > 0 {
		b.WriteString("Available favorites: " + strings.Join(ac.Favorites, ", ") + "\n")
	}
	if ac.TodaysActivity != "" {
		fmt.Fprintf(&b, "Today's activity so far: %s\n", ac.TodaysActivity)
	}
	if ac.TodaysStool {
		b.WriteString("Stool already logged for today.\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// AgentStart begins (or resumes) an agent turn for the given user message + images.
// Returns an AgentTurn that the caller inspects: if ToolCalls is non-empty, execute
// them and pass results back to AgentContinue. Otherwise the run is complete.
func (s *Service) AgentStart(ctx context.Context, sess *AgentSession, ac AgentContext, message string, imgs []ImageData) (*AgentTurn, error) {
	client, err := s.getClient(ctx)
	if err != nil {
		return nil, err
	}

	systemInstr := agentSystemPrompt
	if ctxStr := formatAgentContext(ac); ctxStr != "" {
		systemInstr = systemInstr + "\n\n--- Context ---\n" + ctxStr
	}

	var parts []*genai.Part
	for _, img := range imgs {
		parts = append(parts, &genai.Part{
			InlineData: &genai.Blob{MIMEType: img.MIMEType, Data: img.Data},
			MediaResolution: &genai.PartMediaResolution{
				Level: genai.PartMediaResolutionLevelMediaResolutionLow,
			},
		})
	}
	if message != "" {
		parts = append(parts, &genai.Part{Text: message})
	}

	sess.mu.Lock()
	sess.history = append(sess.history, &genai.Content{Role: string(genai.RoleUser), Parts: parts})
	sess.systemIn = systemInstr
	sess.mu.Unlock()

	cfg := &genai.GenerateContentConfig{
		SystemInstruction: buildSystemInstruction(systemInstr),
		Tools:             agentTools(),
		ThinkingConfig:    &genai.ThinkingConfig{ThinkingLevel: genai.ThinkingLevelLow},
	}

	return s.agentGenerate(ctx, client, sess, cfg)
}

// AgentContinue feeds tool results back to the model and gets the next turn.
func (s *Service) AgentContinue(ctx context.Context, sess *AgentSession, results []AgentToolResult, calls []AgentToolCall) (*AgentTurn, error) {
	if len(results) != len(calls) {
		return nil, fmt.Errorf("results length %d != calls length %d", len(results), len(calls))
	}
	client, err := s.getClient(ctx)
	if err != nil {
		return nil, err
	}

	var parts []*genai.Part
	for i, r := range results {
		parts = append(parts, &genai.Part{
			FunctionResponse: &genai.FunctionResponse{
				Name:     calls[i].Name,
				Response: r.Output,
			},
		})
	}
	sess.mu.Lock()
	sess.history = append(sess.history, &genai.Content{Role: string(genai.RoleUser), Parts: parts})
	systemInstr := sess.systemIn
	sess.mu.Unlock()

	cfg := &genai.GenerateContentConfig{
		SystemInstruction: buildSystemInstruction(systemInstr),
		Tools:             agentTools(),
		ThinkingConfig:    &genai.ThinkingConfig{ThinkingLevel: genai.ThinkingLevelLow},
	}
	return s.agentGenerate(ctx, client, sess, cfg)
}

func (s *Service) agentGenerate(ctx context.Context, client *genai.Client, sess *AgentSession, cfg *genai.GenerateContentConfig) (*AgentTurn, error) {
	sess.mu.Lock()
	contents := make([]*genai.Content, len(sess.history))
	copy(contents, sess.history)
	sess.mu.Unlock()

	resp, err := client.Models.GenerateContent(ctx, geminiModel, contents, cfg)
	if err != nil {
		return nil, fmt.Errorf("agent generate: %w", err)
	}
	if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
		return nil, fmt.Errorf("empty response")
	}
	modelContent := resp.Candidates[0].Content

	sess.mu.Lock()
	sess.history = append(sess.history, modelContent)
	sess.mu.Unlock()

	turn := &AgentTurn{}
	for _, p := range modelContent.Parts {
		if p == nil {
			continue
		}
		if p.FunctionCall != nil {
			turn.ToolCalls = append(turn.ToolCalls, AgentToolCall{
				Name: p.FunctionCall.Name,
				Args: p.FunctionCall.Args,
			})
		}
		if p.Text != "" && !p.Thought {
			if turn.Message != "" {
				turn.Message += "\n"
			}
			turn.Message += p.Text
		}
	}
	return turn, nil
}

// agentSessions holds per-user agent sessions keyed by userEmail|date.
// They live for the duration of a drawer interaction. The handler calls
// ResetAgent when the conversation is complete (e.g. after a successful
// log_meal that the user confirms by closing the drawer).
type agentSessionStore struct {
	mu       sync.Mutex
	sessions map[string]*AgentSession
}

var sessionStores sync.Map // *Service -> *agentSessionStore

func (s *Service) sessionStore() *agentSessionStore {
	if v, ok := sessionStores.Load(s); ok {
		return v.(*agentSessionStore)
	}
	st := &agentSessionStore{sessions: make(map[string]*AgentSession)}
	actual, _ := sessionStores.LoadOrStore(s, st)
	return actual.(*agentSessionStore)
}

// GetOrCreateAgentSession returns the agent session for the given key.
func (s *Service) GetOrCreateAgentSession(key string) *AgentSession {
	st := s.sessionStore()
	st.mu.Lock()
	defer st.mu.Unlock()
	sess, ok := st.sessions[key]
	if !ok {
		sess = &AgentSession{}
		st.sessions[key] = sess
	}
	return sess
}

// ResetAgentSession discards conversation state for the given key.
func (s *Service) ResetAgentSession(key string) {
	st := s.sessionStore()
	st.mu.Lock()
	delete(st.sessions, key)
	st.mu.Unlock()
}

// MarshalToolArgs converts tool args to typed JSON.
func MarshalToolArgs(args map[string]any, out any) error {
	b, err := json.Marshal(args)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}
