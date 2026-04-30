package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"google.golang.org/genai"
)

// agentSessionTTL bounds how long an idle agent session stays in memory.
// Drawer interactions are short; anything beyond this is stale state we'd
// rather drop than carry into a new conversation.
const agentSessionTTL = 30 * time.Minute

// AgentSystemPrompt is the system instruction used for the chat-drawer agent.
// Exported so the frontend can render it for users curious about the prompt.
const AgentSystemPrompt = agentSystemPrompt

const agentSystemPrompt = `You are the user's food + daily log assistant. They're in a single chat drawer and might want to: add/edit/scale/repeat meals, log activity, stool, hydration, or feelings, save a favorite, or just ask questions about their log.

Pick the right tool based on intent. If no tool fits, reply in plain text.

Meals:
- meal_type ∈ {breakfast, snack, lunch, dinner, supplements}. Use "supplements" for vitamins/protein powders.
- All numeric macros are integers (round estimates fine). Fiber: 0 if unknown.
- "edit_meal" replaces the meal currently being edited — return the FULL replacement entry list (omit removed items, include unchanged items unchanged). Only call edit_meal when the context says "Currently editing: <meal>" AND lists current entries for that meal. There is no tool to switch the selected meal. If the user refers to an existing meal but the current context is not editing it, ask them to open that meal or use log_meal for new food.
- "log_meal": if the user is editing a specific meal (context shows "Currently editing"), default to that meal_type; else infer from the user's wording or time of day.
- If the user gives a clock time ("had lunch at 12:30", "around 7pm"), pass it via the optional "time" arg as 24h HH:MM. This anchors the entry on the timeline.
- When logging a meal for a date OTHER than today, always pass "time" — pick a sensible clock time for the meal (breakfast ~08:00, lunch ~12:30, snack ~15:00, dinner ~18:30, supplements ~09:00) unless the user mentions one. Otherwise the entry gets stamped with the current time, which is wrong for retroactive logs.
- If the user says "same as yesterday's <meal>" or "repeat my <meal> from yesterday", look at "Yesterday's meals" and call log_meal with those exact items.
- If the user references a favorite by name (or asks to "add my usual <thing>"), look at "Available favorites" and call log_meal using that favorite's macros and meal_type.
- "Scale" / portion-size requests ("make it 1.5x", "double the rice", "half the portion", "I had two plates of this", "actually it was a bigger serving"): if the context shows "Currently editing: <meal>", you MUST call edit_meal with the SAME items, only with macros multiplied — do NOT call log_meal (that would create a duplicate meal alongside the existing one). If not editing, use log_meal. Round to integers.
- If a photo is provided, estimate from the image — don't ask about anything visible. Only ask ONE clarifying question if quantities are genuinely impossible to tell.
- "add_favorite" saves a meal item for later quick re-logging. Use when the user explicitly asks to save/favorite something.

Daily log (events):
- "log_event" creates a single timeline event. kind ∈ {workout, stool, water, feeling}.
  - workout: text = description (e.g. "30min run"). num optional (minutes).
  - stool: text = optional notes. num unused.
  - water: num = millilitres for THIS event (not a running total — each glass is its own event). text optional.
  - feeling: num = score 1–10 (omit for score-less note). text = optional free-text notes.
- Each event is point-in-time. If the user retroactively reports something, pass "time" as 24h HH:MM. Otherwise omit and the current time is used.
- Don't merge events. "Had another glass of water" is a NEW water event.
- "edit_event": pass the event's id (visible in context as "Today's events") with the fields to change.

Questions:
- Don't call read_log unless the user asks something that needs data not already in context (today's meals, yesterday's meals, today's events, profile, favorites are all in context already).

Style:
- Keep replies brief and conversational. After a successful tool call, a one-line confirmation is enough ("Logged.", "Got it — 320 cal.", "Updated lunch.").
- No filler ("I'd be happy to..."). No emoji unless the user uses one first.`

// agentTools returns the function declarations available to the agent.
// The schema is identical across calls, so build it once.
var agentTools = sync.OnceValue(func() []*genai.Tool {
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
						"time": {
							Type:        genai.TypeString,
							Description: "Optional 24h time (HH:MM) when the meal occurred. Use when the user retroactively logs a meal (e.g. 'I had lunch at 12:30'). Omit to use the current time.",
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
				Name:        "log_event",
				Description: "Create one timeline event. Event kinds: workout (text=desc, num=optional minutes), stool (text=optional notes), water (num=millilitres for this glass), feeling (num=1-10 score, text=optional notes).",
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"kind": {Type: genai.TypeString, Description: "workout | stool | water | feeling"},
						"text": {Type: genai.TypeString, Description: "Optional text per kind (workout description, stool notes, feeling notes)"},
						"num":  {Type: genai.TypeNumber, Description: "Optional numeric per kind (workout minutes, water millilitres, feeling score 1-10)"},
						"time": {Type: genai.TypeString, Description: "Optional 24h time HH:MM if retroactive. Omit for now."},
					},
					Required: []string{"kind"},
				},
			},
			{
				Name:        "edit_event",
				Description: "Edit an existing timeline event by id. Pass only the fields to change.",
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"id":   {Type: genai.TypeString},
						"text": {Type: genai.TypeString},
						"num":  {Type: genai.TypeNumber},
						"time": {Type: genai.TypeString},
					},
					Required: []string{"id"},
				},
			},
			{
				Name:        "delete_event",
				Description: "Delete a timeline event by id.",
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"id": {Type: genai.TypeString},
					},
					Required: []string{"id"},
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
})

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
	lastUsed time.Time
}

// AgentEvent is a minimal event shape for agent context.
// Num is overloaded by Kind: workout=duration_min, water=millilitres,
// feeling=score 1-10, stool=unused.
type AgentEvent struct {
	ID    string
	Time  string
	Kind  string
	Text  string
	Num   float64
}

// AgentContext describes the drawer state passed to the agent each turn.
type AgentContext struct {
	Date            string             // YYYY-MM-DD currently being viewed
	SelectedMeal    string             // breakfast/lunch/etc or ""
	CurrentEntries  []Entry            // entries in selected meal (for edit_meal)
	YesterdayByMeal map[string][]Entry // for "same as yesterday's lunch"
	TodayByMeal     map[string][]Entry // current day, all meals — for scaling/repeating without selecting
	Favorites       []FavoriteRef      // model can match by name and use macros
	Profile         string             // pre-formatted profile context
	TodaysEvents    []AgentEvent       // workout/stool/water/feeling events for current date
	Extra           map[string]any
}

// FavoriteRef is a minimal favorite shape for agent context.
type FavoriteRef struct {
	Description string
	MealType    string
	Calories    int
	Protein     int
	Carbs       int
	Fat         int
	Fiber       int
}

// mealOrder is the canonical ordering for meal sections in agent context.
// Map iteration would otherwise vary the prompt across calls for identical state.
var mealOrder = []string{"breakfast", "snack", "lunch", "dinner", "supplements"}

func writeMealMap(b *strings.Builder, m map[string][]Entry) {
	seen := make(map[string]bool, len(m))
	write := func(meal string, entries []Entry) {
		if len(entries) == 0 {
			return
		}
		fmt.Fprintf(b, "  %s:\n", meal)
		for _, e := range entries {
			fmt.Fprintf(b, "    - %s (%dcal, %dgP, %dgC, %dgF, %dgFib)\n",
				e.Description, e.Calories, e.Protein, e.Carbs, e.Fat, e.Fiber)
		}
	}
	for _, meal := range mealOrder {
		write(meal, m[meal])
		seen[meal] = true
	}
	extras := make([]string, 0)
	for k := range m {
		if !seen[k] {
			extras = append(extras, k)
		}
	}
	sort.Strings(extras)
	for _, k := range extras {
		write(k, m[k])
	}
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
		fmt.Fprintf(&b, "Currently editing: %s\n", ac.SelectedMeal)
	}
	if len(ac.CurrentEntries) > 0 {
		b.WriteString("Current entries in the meal being edited:\n")
		for _, e := range ac.CurrentEntries {
			fmt.Fprintf(&b, "  - %s (%dcal, %dgP, %dgC, %dgF, %dgFib)\n",
				e.Description, e.Calories, e.Protein, e.Carbs, e.Fat, e.Fiber)
		}
	}
	if len(ac.TodayByMeal) > 0 {
		b.WriteString("Today's meals so far:\n")
		writeMealMap(&b, ac.TodayByMeal)
	}
	if len(ac.YesterdayByMeal) > 0 {
		b.WriteString("Yesterday's meals:\n")
		writeMealMap(&b, ac.YesterdayByMeal)
	}
	if len(ac.Favorites) > 0 {
		b.WriteString("Available favorites:\n")
		for _, f := range ac.Favorites {
			fmt.Fprintf(&b, "  - %s [%s] (%dcal, %dgP, %dgC, %dgF, %dgFib)\n",
				f.Description, f.MealType, f.Calories, f.Protein, f.Carbs, f.Fat, f.Fiber)
		}
	}
	if len(ac.TodaysEvents) > 0 {
		b.WriteString("Today's events:\n")
		for _, ev := range ac.TodaysEvents {
			fmt.Fprintf(&b, "  - id=%s [%s] %s", ev.ID, ev.Time, ev.Kind)
			if ev.Text != "" {
				fmt.Fprintf(&b, " text=%q", ev.Text)
			}
			if ev.Num != 0 {
				fmt.Fprintf(&b, " num=%v", ev.Num)
			}
			b.WriteString("\n")
		}
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

// agentSessionStore holds per-user agent sessions keyed by userEmail|date.
// Idle sessions older than agentSessionTTL are evicted on every access so the
// map can't grow without bound as users navigate dates.
type agentSessionStore struct {
	mu       sync.Mutex
	sessions map[string]*AgentSession
}

func (s *Service) sessionStoreOnce() *agentSessionStore {
	s.agentInit.Do(func() {
		s.agentStore = &agentSessionStore{sessions: make(map[string]*AgentSession)}
	})
	return s.agentStore
}

// evictExpiredLocked removes sessions whose lastUsed is older than agentSessionTTL.
// Caller must hold st.mu.
func (st *agentSessionStore) evictExpiredLocked(now time.Time) {
	for k, sess := range st.sessions {
		if now.Sub(sess.lastUsed) > agentSessionTTL {
			delete(st.sessions, k)
		}
	}
}

// GetOrCreateAgentSession returns the agent session for the given key, evicting
// any other sessions that have gone idle past agentSessionTTL.
func (s *Service) GetOrCreateAgentSession(key string) *AgentSession {
	st := s.sessionStoreOnce()
	st.mu.Lock()
	defer st.mu.Unlock()
	now := time.Now()
	st.evictExpiredLocked(now)
	sess, ok := st.sessions[key]
	if !ok {
		sess = &AgentSession{lastUsed: now}
		st.sessions[key] = sess
	} else {
		sess.lastUsed = now
	}
	return sess
}

// ResetAgentSession discards conversation state for the given key.
func (s *Service) ResetAgentSession(key string) {
	st := s.sessionStoreOnce()
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
