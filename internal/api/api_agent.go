package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"foodtracker/internal/auth"
	"foodtracker/internal/gemini"
	"foodtracker/internal/sheets"
)

const maxAgentIterations = 6

// agentRequest is the input shape to /api/agent.
type agentRequest struct {
	Message        string             `json:"message"`
	Date           string             `json:"date"`
	Meal           string             `json:"meal"`
	Time           string             `json:"time"`
	CurrentEntries []sheets.FoodEntry `json:"current_entries"`
	Reset          bool               `json:"reset"`
	Images         []gemini.ImageData `json:"-"`
}

// AgentAction is one observable side-effect the agent performed during this turn.
// The frontend uses these to refresh affected UI without re-fetching everything.
type AgentAction struct {
	Type    string             `json:"type"` // "meal_added" | "meal_edited" | "event_added" | "event_edited" | "event_deleted" | "favorite_added"
	Entries []sheets.FoodEntry `json:"entries,omitempty"`
	Removed []string           `json:"removed_ids,omitempty"`
	Date    string             `json:"date,omitempty"`
	Event   *sheets.Event      `json:"event,omitempty"`
	EventID string             `json:"event_id,omitempty"`
}

// agentResponse is the output of /api/agent for one user message.
type agentResponse struct {
	Message string        `json:"message"`
	Actions []AgentAction `json:"actions"`
}

// POST /api/agent
func (h *Handler) Agent(c *echo.Context) error {
	session := auth.SessionFrom(c)

	r := c.Request()
	ctx := r.Context()
	req, err := parseAgentRequest(r)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return writeErr(c, http.StatusRequestEntityTooLarge, "upload_too_large")
		}
		return writeErr(c, http.StatusBadRequest, "invalid request body")
	}
	if strings.TrimSpace(req.Message) == "" && len(req.Images) == 0 {
		return writeErr(c, http.StatusBadRequest, "message or image required")
	}

	targetDate := req.Date
	if targetDate == "" {
		targetDate = sheets.DateString(LocalNow(r))
	}

	svc, err := h.sheetsSvc(c, session)
	if err != nil {
		return h.writeAPIErr(c, err)
	}

	// Build context: profile, yesterday's meals, favorites, today's activity.
	profileCacheKey := session.SpreadsheetID + "|profile"
	var profileCtx string
	if cached, ok := h.cacheGet(profileCacheKey); ok {
		profileCtx = string(cached)
	} else {
		profile, _ := svc.GetProfile(ctx)
		profileCtx = formatProfileContext(profile, LocalNow(r).Year())
		h.cacheSet(profileCacheKey, []byte(profileCtx))
	}

	yesterday := sheets.DateString(LocalNow(r).AddDate(0, 0, -1))
	yEntries, _ := svc.GetFoodByDate(ctx, yesterday)
	yesterdayByMeal := groupByMeal(yEntries)

	tEntries, _ := svc.GetFoodByDate(ctx, targetDate)
	todayByMeal := groupByMeal(tEntries)

	favs, _ := svc.GetFavorites(ctx)
	favRefs := make([]gemini.FavoriteRef, 0, len(favs))
	for _, f := range favs {
		favRefs = append(favRefs, gemini.FavoriteRef{
			Description: f.Description, MealType: f.MealType,
			Calories: f.Calories, Protein: f.Protein, Carbs: f.Carbs,
			Fat: f.Fat, Fiber: f.Fiber,
		})
	}

	todaysEvents, _ := svc.GetEventsByDate(ctx, targetDate)
	agentEvents := make([]gemini.AgentEvent, 0, len(todaysEvents))
	for _, ev := range todaysEvents {
		agentEvents = append(agentEvents, gemini.AgentEvent{
			ID: ev.ID, Time: ev.Time, Kind: ev.Kind, Text: ev.Text, Num: ev.Num,
		})
	}

	current := make([]gemini.Entry, 0, len(req.CurrentEntries))
	for _, e := range req.CurrentEntries {
		current = append(current, gemini.Entry{
			MealType:    e.MealType,
			Description: e.Description,
			Calories:    e.Calories,
			Protein:     e.Protein,
			Carbs:       e.Carbs,
			Fat:         e.Fat,
			Fiber:       e.Fiber,
		})
	}

	ac := gemini.AgentContext{
		Date:            targetDate,
		SelectedMeal:    req.Meal,
		CurrentEntries:  current,
		YesterdayByMeal: convertMealMap(yesterdayByMeal),
		TodayByMeal:     convertMealMap(todayByMeal),
		Favorites:       favRefs,
		Profile:         profileCtx,
		TodaysEvents:    agentEvents,
	}

	sessionKey := session.UserEmail + "|" + targetDate
	if req.Reset {
		h.gemini.ResetAgentSession(sessionKey)
	}
	agentSess := h.gemini.GetOrCreateAgentSession(sessionKey)

	turn, err := h.gemini.AgentStart(ctx, agentSess, ac, req.Message, req.Images)
	if err != nil {
		return writeErr(c, http.StatusInternalServerError, "agent error: "+err.Error())
	}

	executor := &agentExecutor{
		handler:         h,
		ctx:             ctx,
		svc:             svc,
		session:         session,
		date:            targetDate,
		mealCtx:         req.Meal,
		userMealTime:    strings.TrimSpace(req.Time),
		currentEntries:  req.CurrentEntries,
		todaysEvents:    todaysEvents,
		now:             LocalNow(r),
	}

	// Tool-call loop. If we exit with pending tool calls, the model wanted to
	// keep going but we capped it — surface that rather than silently dropping
	// the unfinished work and returning a stale Message.
	for range maxAgentIterations {
		if len(turn.ToolCalls) == 0 {
			break
		}
		results := make([]gemini.AgentToolResult, len(turn.ToolCalls))
		for j, call := range turn.ToolCalls {
			out := executor.execute(call)
			results[j] = gemini.AgentToolResult{Output: out}
		}
		next, err := h.gemini.AgentContinue(ctx, agentSess, results, turn.ToolCalls)
		if err != nil {
			return writeErr(c, http.StatusInternalServerError, "agent error: "+err.Error())
		}
		turn = next
	}
	if len(turn.ToolCalls) > 0 {
		// Reset the session — its history now ends on a model turn with unsent
		// tool results, which would confuse the next request.
		h.gemini.ResetAgentSession(sessionKey)
		return writeErr(c, http.StatusInternalServerError,
			fmt.Sprintf("agent exceeded %d tool-call iterations", maxAgentIterations))
	}

	// If any side-effects occurred, invalidate caches and clear convo on terminal action.
	if len(executor.actions) > 0 {
		h.cacheInvalidate(session.SpreadsheetID)
	}

	return c.JSON(http.StatusOK, agentResponse{
		Message: strings.TrimSpace(turn.Message),
		Actions: executor.actions,
	})
}

// agentExecutor wires the agent's tool calls to the sheets service and tracks side-effects.
type agentExecutor struct {
	handler        *Handler
	ctx            context.Context
	svc            *sheets.Service
	session        *auth.Session
	date           string
	mealCtx        string
	userMealTime   string
	currentEntries []sheets.FoodEntry
	todaysEvents   []sheets.Event
	now            time.Time
	actions        []AgentAction
}

func (ex *agentExecutor) execute(call gemini.AgentToolCall) map[string]any {
	switch call.Name {
	case "log_meal":
		return ex.logMeal(call.Args)
	case "edit_meal":
		return ex.editMeal(call.Args)
	case "log_event":
		return ex.logEvent(call.Args)
	case "edit_event":
		return ex.editEvent(call.Args)
	case "delete_event":
		return ex.deleteEvent(call.Args)
	case "add_favorite":
		return ex.addFavorite(call.Args)
	case "read_log":
		return ex.readLog(call.Args)
	default:
		return map[string]any{"error": "unknown tool: " + call.Name}
	}
}

func (ex *agentExecutor) logMeal(args map[string]any) map[string]any {
	var p struct {
		MealType string              `json:"meal_type"`
		Items    []gemini.AgentEntry `json:"items"`
		Time     string              `json:"time"`
	}
	if err := gemini.MarshalToolArgs(args, &p); err != nil {
		return map[string]any{"error": "invalid args: " + err.Error()}
	}
	if len(p.Items) == 0 {
		return map[string]any{"error": "items required"}
	}
	timeStr := resolveLogMealTime(ex.now, ex.date, p.MealType, ex.userMealTime, p.Time)
	saved := make([]sheets.FoodEntry, 0, len(p.Items))
	for _, it := range p.Items {
		fe := sheets.FoodEntry{
			ID: uuid.NewString(), Date: ex.date, Time: timeStr,
			MealType: p.MealType, Description: it.Description,
			Calories: it.Calories, Protein: it.Protein,
			Carbs: it.Carbs, Fat: it.Fat, Fiber: it.Fiber,
		}
		ge := gemini.Entry{
			MealType: fe.MealType, Description: fe.Description,
			Calories: fe.Calories, Protein: fe.Protein,
			Carbs: fe.Carbs, Fat: fe.Fat, Fiber: fe.Fiber,
		}
		if err := ge.Validate(); err != nil {
			return map[string]any{"error": "invalid entry: " + err.Error()}
		}
		saved = append(saved, fe)
	}
	if err := ex.svc.AppendFoods(ex.ctx, saved); err != nil {
		return map[string]any{"error": "sheet write: " + err.Error()}
	}
	ex.actions = append(ex.actions, AgentAction{
		Type: "meal_added", Entries: saved, Date: ex.date,
	})
	return map[string]any{"status": "logged", "count": len(saved)}
}

func (ex *agentExecutor) editMeal(args map[string]any) map[string]any {
	var p struct {
		Items []gemini.AgentEntry `json:"items"`
	}
	if err := gemini.MarshalToolArgs(args, &p); err != nil {
		return map[string]any{"error": "invalid args: " + err.Error()}
	}
	if len(ex.currentEntries) == 0 && len(p.Items) == 0 {
		return map[string]any{"error": "no current entries to edit"}
	}
	mealType := ex.mealCtx
	if mealType == "" && len(ex.currentEntries) > 0 {
		mealType = ex.currentEntries[0].MealType
	}

	// Pair new items to existing entries by description, but allow each old
	// entry to be claimed at most once so that two identical descriptions in
	// the new list don't both update (or both skip) the same row.
	usedIDs := map[string]bool{}
	claimByDesc := func(desc string) (sheets.FoodEntry, bool) {
		for _, e := range ex.currentEntries {
			if e.Description == desc && !usedIDs[e.ID] {
				usedIDs[e.ID] = true
				return e, true
			}
		}
		return sheets.FoodEntry{}, false
	}

	timeStr := sheets.TimeString(ex.now)
	saved := []sheets.FoodEntry{}
	toAppend := []sheets.FoodEntry{}

	for _, it := range p.Items {
		ge := gemini.Entry{
			MealType: mealType, Description: it.Description,
			Calories: it.Calories, Protein: it.Protein,
			Carbs: it.Carbs, Fat: it.Fat, Fiber: it.Fiber,
		}
		if err := ge.Validate(); err != nil {
			return map[string]any{"error": "invalid entry: " + err.Error()}
		}
		if old, ok := claimByDesc(it.Description); ok {
			fe := old
			fe.MealType = mealType
			fe.Calories = it.Calories
			fe.Protein = it.Protein
			fe.Carbs = it.Carbs
			fe.Fat = it.Fat
			fe.Fiber = it.Fiber
			if err := ex.svc.UpdateFood(ex.ctx, fe.ID, fe); err != nil {
				return map[string]any{"error": "sheet update: " + err.Error()}
			}
			saved = append(saved, fe)
		} else {
			fe := sheets.FoodEntry{
				ID: uuid.NewString(), Date: ex.date, Time: timeStr,
				MealType: mealType, Description: it.Description,
				Calories: it.Calories, Protein: it.Protein,
				Carbs: it.Carbs, Fat: it.Fat, Fiber: it.Fiber,
			}
			toAppend = append(toAppend, fe)
			saved = append(saved, fe)
		}
	}
	if err := ex.svc.AppendFoods(ex.ctx, toAppend); err != nil {
		return map[string]any{"error": "sheet write: " + err.Error()}
	}

	var removed []string
	for _, e := range ex.currentEntries {
		if !usedIDs[e.ID] {
			_ = ex.svc.DeleteFood(ex.ctx, e.ID)
			removed = append(removed, e.ID)
		}
	}

	// Update local state so subsequent tool calls see the new entries.
	ex.currentEntries = saved
	ex.actions = append(ex.actions, AgentAction{
		Type: "meal_edited", Entries: saved, Removed: removed, Date: ex.date,
	})
	return map[string]any{"status": "edited", "kept": len(saved), "removed": len(removed)}
}

// resolveLogMealTime returns the HH:MM string to stamp a logged meal with,
// applying this precedence (highest wins): tool-supplied time → user-supplied
// time → defaultMealTime when retroactive → current clock time. Invalid
// HH:MM inputs at any layer are ignored (fall through to the next source).
func resolveLogMealTime(now time.Time, date, mealType, userMealTime, toolTime string) string {
	timeStr := sheets.TimeString(now)
	if date != sheets.DateString(now) {
		timeStr = defaultMealTime(mealType)
	}
	if userMealTime != "" {
		if parsed, err := time.Parse("15:04", userMealTime); err == nil {
			timeStr = parsed.Format("15:04")
		}
	}
	if t := strings.TrimSpace(toolTime); t != "" {
		if parsed, err := time.Parse("15:04", t); err == nil {
			timeStr = parsed.Format("15:04")
		}
	}
	return timeStr
}

// defaultMealTime returns a conventional clock time for a meal type, used when
// logging a meal for a past date and the agent doesn't supply an explicit time.
// Without this, retroactive entries get stamped with the current server time
// (e.g. logging yesterday's dinner at 7am gives it a 7am timestamp).
func defaultMealTime(mealType string) string {
	switch mealType {
	case "breakfast":
		return "08:00"
	case "lunch":
		return "12:30"
	case "snack":
		return "15:00"
	case "dinner":
		return "18:30"
	case "supplements":
		return "09:00"
	default:
		return "12:00"
	}
}

func validEventKind(k string) bool {
	switch k {
	case sheets.EventKindWorkout, sheets.EventKindStool, sheets.EventKindWater, sheets.EventKindFeeling:
		return true
	}
	return false
}

func (ex *agentExecutor) logEvent(args map[string]any) map[string]any {
	var p struct {
		Kind string  `json:"kind"`
		Text string  `json:"text"`
		Num  float64 `json:"num"`
		Time string  `json:"time"`
	}
	if err := gemini.MarshalToolArgs(args, &p); err != nil {
		return map[string]any{"error": "invalid args: " + err.Error()}
	}
	if !validEventKind(p.Kind) {
		return map[string]any{"error": "kind must be workout|stool|water|feeling"}
	}
	timeStr := sheets.TimeString(ex.now)
	if t := strings.TrimSpace(p.Time); t != "" {
		if parsed, err := time.Parse("15:04", t); err == nil {
			timeStr = parsed.Format("15:04")
		}
	}
	if p.Kind == sheets.EventKindFeeling && p.Num != 0 && (p.Num < 1 || p.Num > 10) {
		return map[string]any{"error": "feeling score must be 1-10"}
	}
	if p.Kind == sheets.EventKindWater && (p.Num < 0 || p.Num > 5000) {
		return map[string]any{"error": "water millilitres out of range"}
	}
	ev := sheets.Event{
		ID: uuid.NewString(), Date: ex.date, Time: timeStr,
		Kind: p.Kind, Text: strings.TrimSpace(p.Text), Num: p.Num,
	}
	if err := ex.svc.AppendEvent(ex.ctx, ev); err != nil {
		return map[string]any{"error": "sheet write: " + err.Error()}
	}
	ex.todaysEvents = append(ex.todaysEvents, ev)
	evCopy := ev
	ex.actions = append(ex.actions, AgentAction{Type: "event_added", Date: ex.date, Event: &evCopy})
	return map[string]any{"status": "logged", "id": ev.ID}
}

func (ex *agentExecutor) findEvent(id string) (sheets.Event, int, bool) {
	for i, ev := range ex.todaysEvents {
		if ev.ID == id {
			return ev, i, true
		}
	}
	return sheets.Event{}, -1, false
}

func (ex *agentExecutor) editEvent(args map[string]any) map[string]any {
	var p struct {
		ID   string   `json:"id"`
		Text *string  `json:"text"`
		Num  *float64 `json:"num"`
		Time *string  `json:"time"`
	}
	if err := gemini.MarshalToolArgs(args, &p); err != nil {
		return map[string]any{"error": "invalid args: " + err.Error()}
	}
	ev, idx, ok := ex.findEvent(p.ID)
	if !ok {
		return map[string]any{"error": "event not found"}
	}
	if p.Text != nil {
		ev.Text = strings.TrimSpace(*p.Text)
	}
	if p.Num != nil {
		ev.Num = *p.Num
	}
	if p.Time != nil {
		if parsed, err := time.Parse("15:04", strings.TrimSpace(*p.Time)); err == nil {
			ev.Time = parsed.Format("15:04")
		}
	}
	if err := ex.svc.UpdateEvent(ex.ctx, ev.ID, ev); err != nil {
		return map[string]any{"error": "sheet write: " + err.Error()}
	}
	ex.todaysEvents[idx] = ev
	evCopy := ev
	ex.actions = append(ex.actions, AgentAction{Type: "event_edited", Date: ex.date, Event: &evCopy})
	return map[string]any{"status": "edited", "id": ev.ID}
}

func (ex *agentExecutor) deleteEvent(args map[string]any) map[string]any {
	var p struct {
		ID string `json:"id"`
	}
	if err := gemini.MarshalToolArgs(args, &p); err != nil {
		return map[string]any{"error": "invalid args: " + err.Error()}
	}
	if _, idx, ok := ex.findEvent(p.ID); ok {
		ex.todaysEvents = append(ex.todaysEvents[:idx], ex.todaysEvents[idx+1:]...)
	}
	if err := ex.svc.DeleteEvent(ex.ctx, p.ID); err != nil {
		return map[string]any{"error": "delete: " + err.Error()}
	}
	ex.actions = append(ex.actions, AgentAction{Type: "event_deleted", Date: ex.date, EventID: p.ID})
	return map[string]any{"status": "deleted"}
}

func (ex *agentExecutor) addFavorite(args map[string]any) map[string]any {
	var p struct {
		Description string `json:"description"`
		MealType    string `json:"meal_type"`
		Calories    int    `json:"calories"`
		Protein     int    `json:"protein"`
		Carbs       int    `json:"carbs"`
		Fat         int    `json:"fat"`
		Fiber       int    `json:"fiber"`
	}
	if err := gemini.MarshalToolArgs(args, &p); err != nil {
		return map[string]any{"error": "invalid args: " + err.Error()}
	}
	if strings.TrimSpace(p.Description) == "" {
		return map[string]any{"error": "description required"}
	}
	existing, _ := ex.svc.GetFavorites(ex.ctx)
	key := sheets.NormalizeFavoriteKey(p.Description)
	for _, e := range existing {
		if sheets.NormalizeFavoriteKey(e.Description) == key {
			return map[string]any{"error": "favorite already exists"}
		}
	}
	fav := sheets.FavoriteEntry{
		ID: uuid.NewString(), Description: p.Description, MealType: p.MealType,
		Calories: p.Calories, Protein: p.Protein, Carbs: p.Carbs, Fat: p.Fat, Fiber: p.Fiber,
		CreatedAt: ex.date,
	}
	if err := ex.svc.AddFavorite(ex.ctx, fav); err != nil {
		return map[string]any{"error": "sheet write: " + err.Error()}
	}
	ex.actions = append(ex.actions, AgentAction{Type: "favorite_added", Date: ex.date})
	return map[string]any{"status": "added"}
}

func (ex *agentExecutor) readLog(args map[string]any) map[string]any {
	var p struct {
		StartDate string `json:"start_date"`
		EndDate   string `json:"end_date"`
	}
	if err := gemini.MarshalToolArgs(args, &p); err != nil {
		return map[string]any{"error": "invalid args: " + err.Error()}
	}
	if p.StartDate == "" {
		return map[string]any{"error": "start_date required"}
	}
	if p.EndDate == "" {
		p.EndDate = p.StartDate
	}
	entries, err := ex.svc.GetFoodByDateRange(ex.ctx, p.StartDate, p.EndDate)
	if err != nil {
		return map[string]any{"error": "read: " + err.Error()}
	}
	events, _ := ex.svc.GetEventsByDateRange(ex.ctx, p.StartDate, p.EndDate)
	return map[string]any{
		"entries":    entries,
		"events":     events,
		"start_date": p.StartDate,
		"end_date":   p.EndDate,
	}
}

func groupByMeal(entries []sheets.FoodEntry) map[string][]sheets.FoodEntry {
	out := map[string][]sheets.FoodEntry{}
	for _, e := range entries {
		out[e.MealType] = append(out[e.MealType], e)
	}
	return out
}

func convertMealMap(m map[string][]sheets.FoodEntry) map[string][]gemini.Entry {
	out := map[string][]gemini.Entry{}
	for k, v := range m {
		for _, e := range v {
			out[k] = append(out[k], gemini.Entry{
				MealType: e.MealType, Description: e.Description,
				Calories: e.Calories, Protein: e.Protein,
				Carbs: e.Carbs, Fat: e.Fat, Fiber: e.Fiber,
			})
		}
	}
	return out
}

func parseAgentRequest(r *http.Request) (agentRequest, error) {
	contentType := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
	if strings.HasPrefix(contentType, "multipart/form-data") {
		return parseAgentMultipart(r)
	}
	return parseAgentJSON(r)
}

func parseAgentJSON(r *http.Request) (agentRequest, error) {
	var raw struct {
		Message        string             `json:"message"`
		Date           string             `json:"date"`
		Meal           string             `json:"meal"`
		Time           string             `json:"time"`
		CurrentEntries []sheets.FoodEntry `json:"current_entries"`
		Reset          bool               `json:"reset"`
		Images         []struct {
			MIMEType string `json:"mime_type"`
			Data     string `json:"data"`
		} `json:"images"`
	}
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		return agentRequest{}, err
	}
	out := agentRequest{
		Message: raw.Message, Date: raw.Date, Meal: raw.Meal, Time: raw.Time,
		CurrentEntries: raw.CurrentEntries, Reset: raw.Reset,
	}
	for _, img := range raw.Images {
		decoded, err := base64.StdEncoding.DecodeString(img.Data)
		if err != nil {
			return agentRequest{}, err
		}
		out.Images = append(out.Images, gemini.ImageData{MIMEType: img.MIMEType, Data: decoded})
	}
	return out, nil
}

func parseAgentMultipart(r *http.Request) (agentRequest, error) {
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		return agentRequest{}, err
	}
	out := agentRequest{
		Message: r.FormValue("message"),
		Date:    r.FormValue("date"),
		Meal:    r.FormValue("meal"),
		Time:    r.FormValue("time"),
		Reset:   r.FormValue("reset") == "true",
	}
	if raw := r.FormValue("current_entries"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &out.CurrentEntries); err != nil {
			return agentRequest{}, fmt.Errorf("current_entries: %w", err)
		}
	}
	for _, field := range []string{"images", "image"} {
		files := r.MultipartForm.File[field]
		for _, fh := range files {
			file, err := fh.Open()
			if err != nil {
				return agentRequest{}, err
			}
			data, readErr := io.ReadAll(file)
			closeErr := file.Close()
			if readErr != nil {
				return agentRequest{}, readErr
			}
			if closeErr != nil {
				return agentRequest{}, closeErr
			}
			if len(data) == 0 {
				continue
			}
			mimeType := strings.TrimSpace(fh.Header.Get("Content-Type"))
			if mimeType == "" {
				mimeType = http.DetectContentType(data)
			}
			if !strings.HasPrefix(strings.ToLower(mimeType), "image/") {
				return agentRequest{}, errors.New("invalid image upload")
			}
			out.Images = append(out.Images, gemini.ImageData{MIMEType: mimeType, Data: data})
		}
	}
	return out, nil
}
