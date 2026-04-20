package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"foodtracker/internal/auth"
	"foodtracker/internal/sheets"
)

// POST /api/insights
func (h *Handler) Insights(c *echo.Context) error {
	session := auth.SessionFrom(c)

	r := c.Request()
	ctx := r.Context()
	var req struct {
		Start string `json:"start"`
		End   string `json:"end"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Start == "" || req.End == "" {
		return writeErr(c, http.StatusBadRequest, "start and end dates required")
	}
	if _, err := time.Parse("2006-01-02", req.Start); err != nil {
		return writeErr(c, http.StatusBadRequest, "invalid start date")
	}
	if _, err := time.Parse("2006-01-02", req.End); err != nil {
		return writeErr(c, http.StatusBadRequest, "invalid end date")
	}
	startT, _ := time.Parse("2006-01-02", req.Start)
	endT, _ := time.Parse("2006-01-02", req.End)
	if endT.Sub(startT) > 31*24*time.Hour {
		return writeErr(c, http.StatusBadRequest, "date range too large")
	}

	svc, err := h.sheetsSvc(c, session)
	if err != nil {
		return h.writeAPIErr(c, err)
	}

	entries, err := svc.GetFoodByDateRange(ctx, req.Start, req.End)
	if err != nil {
		return h.writeAPIErr(c, err)
	}
	dailyLogs, err := svc.GetActivityByDateRange(ctx, req.Start, req.End)
	if err != nil {
		return h.writeAPIErr(c, err)
	}

	summary := buildWeekSummary(req.Start, req.End, entries, dailyLogs)

	profileCacheKey := session.SpreadsheetID + "|profile"
	var profileCtx string
	if cached, ok := h.cacheGet(profileCacheKey); ok {
		profileCtx = string(cached)
	} else {
		profile, _ := svc.GetProfile(ctx)
		profileCtx = formatProfileContext(profile, LocalNow(r).Year())
		h.cacheSet(profileCacheKey, []byte(profileCtx))
	}

	insight, err := h.gemini.Insights(ctx, summary, profileCtx)
	if err != nil {
		return writeErr(c, http.StatusInternalServerError, "gemini error: "+err.Error())
	}
	generatedAt := time.Now().UTC().Format(time.RFC3339)
	_ = svc.SaveInsight(ctx, sheets.InsightRecord{
		Type:        "week",
		StartDate:   req.Start,
		EndDate:     req.End,
		GeneratedAt: generatedAt,
		Insight:     insight,
	})
	return c.JSON(http.StatusOK, map[string]any{"insight": insight, "generated_at": generatedAt})
}

// POST /api/insights/day
func (h *Handler) DayInsights(c *echo.Context) error {
	session := auth.SessionFrom(c)

	r := c.Request()
	ctx := r.Context()
	var req struct {
		Date string `json:"date"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Date == "" {
		return writeErr(c, http.StatusBadRequest, "date required")
	}
	if _, err := time.Parse("2006-01-02", req.Date); err != nil {
		return writeErr(c, http.StatusBadRequest, "invalid date")
	}

	svc, err := h.sheetsSvc(c, session)
	if err != nil {
		return h.writeAPIErr(c, err)
	}

	entries, err := svc.GetFoodByDateRange(ctx, req.Date, req.Date)
	if err != nil {
		return h.writeAPIErr(c, err)
	}
	dailyLogs, err := svc.GetActivityByDateRange(ctx, req.Date, req.Date)
	if err != nil {
		return h.writeAPIErr(c, err)
	}

	if len(entries) == 0 && len(dailyLogs) == 0 {
		return writeErr(c, http.StatusBadRequest, "no data for this day")
	}

	today := sheets.DateString(LocalNow(r))
	summary := buildDaySummary(req.Date, entries, dailyLogs, req.Date == today)

	profileCacheKey := session.SpreadsheetID + "|profile"
	var profileCtx string
	if cached, ok := h.cacheGet(profileCacheKey); ok {
		profileCtx = string(cached)
	} else {
		profile, _ := svc.GetProfile(ctx)
		profileCtx = formatProfileContext(profile, LocalNow(r).Year())
		h.cacheSet(profileCacheKey, []byte(profileCtx))
	}

	insight, err := h.gemini.DayInsights(ctx, summary, profileCtx)
	if err != nil {
		return writeErr(c, http.StatusInternalServerError, "gemini error: "+err.Error())
	}
	generatedAt := time.Now().UTC().Format(time.RFC3339)
	_ = svc.SaveInsight(ctx, sheets.InsightRecord{
		Type:        "day",
		StartDate:   req.Date,
		EndDate:     req.Date,
		GeneratedAt: generatedAt,
		Insight:     insight,
	})
	return c.JSON(http.StatusOK, map[string]any{"insight": insight, "generated_at": generatedAt})
}

// GET /api/insights?start=...&end=...
func (h *Handler) GetStoredInsights(c *echo.Context) error {
	session := auth.SessionFrom(c)

	start := c.QueryParam("start")
	end := c.QueryParam("end")
	if start == "" || end == "" {
		return writeErr(c, http.StatusBadRequest, "start and end required")
	}
	svc, err := h.sheetsSvc(c, session)
	if err != nil {
		return h.writeAPIErr(c, err)
	}
	rec, err := svc.GetInsight(c.Request().Context(), "week", start, end)
	if err != nil {
		return h.writeAPIErr(c, err)
	}
	if rec == nil {
		return c.JSON(http.StatusOK, map[string]any{"insight": nil, "generated_at": nil})
	}
	return c.JSON(http.StatusOK, map[string]any{"insight": rec.Insight, "generated_at": rec.GeneratedAt})
}

// GET /api/insights/day?date=...
func (h *Handler) GetStoredDayInsights(c *echo.Context) error {
	session := auth.SessionFrom(c)

	date := c.QueryParam("date")
	if date == "" {
		return writeErr(c, http.StatusBadRequest, "date required")
	}
	svc, err := h.sheetsSvc(c, session)
	if err != nil {
		return h.writeAPIErr(c, err)
	}
	rec, err := svc.GetInsight(c.Request().Context(), "day", date, date)
	if err != nil {
		return h.writeAPIErr(c, err)
	}
	if rec == nil {
		return c.JSON(http.StatusOK, map[string]any{"insight": nil, "generated_at": nil})
	}
	return c.JSON(http.StatusOK, map[string]any{"insight": rec.Insight, "generated_at": rec.GeneratedAt})
}

// POST /api/suggestions/day
func (h *Handler) DaySuggestions(c *echo.Context) error {
	session := auth.SessionFrom(c)

	r := c.Request()
	ctx := r.Context()
	var req struct {
		Date string `json:"date"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Date == "" {
		return writeErr(c, http.StatusBadRequest, "date required")
	}
	if _, err := time.Parse("2006-01-02", req.Date); err != nil {
		return writeErr(c, http.StatusBadRequest, "invalid date")
	}

	svc, err := h.sheetsSvc(c, session)
	if err != nil {
		return h.writeAPIErr(c, err)
	}

	entries, err := svc.GetFoodByDateRange(ctx, req.Date, req.Date)
	if err != nil {
		return h.writeAPIErr(c, err)
	}

	hasMeal := map[string]bool{}
	for _, e := range entries {
		hasMeal[e.MealType] = true
	}
	complete := hasMeal["breakfast"] && hasMeal["lunch"] && hasMeal["dinner"]

	prevDate := addDaysStr(req.Date, -1)
	prevEntries, _ := svc.GetFoodByDateRange(ctx, prevDate, prevDate)

	profileCacheKey := session.SpreadsheetID + "|profile"
	var profileCtx string
	if cached, ok := h.cacheGet(profileCacheKey); ok {
		profileCtx = string(cached)
	} else {
		profile, _ := svc.GetProfile(ctx)
		profileCtx = formatProfileContext(profile, LocalNow(r).Year())
		h.cacheSet(profileCacheKey, []byte(profileCtx))
	}

	var insightText string
	if rec, _ := svc.GetInsight(ctx, "day", req.Date, req.Date); rec != nil {
		insightText = rec.Insight
	} else if rec, _ := svc.GetInsight(ctx, "day", prevDate, prevDate); rec != nil {
		insightText = rec.Insight
	}

	summary := buildMealSuggestionSummary(req.Date, entries, prevEntries, complete, insightText)

	sugType := "remaining"
	if complete {
		sugType = "next-day"
	}

	suggestions, err := h.gemini.MealSuggestions(ctx, summary, profileCtx)
	if err != nil {
		return writeErr(c, http.StatusInternalServerError, "gemini error: "+err.Error())
	}
	generatedAt := time.Now().UTC().Format(time.RFC3339)
	_ = svc.SaveInsight(ctx, sheets.InsightRecord{
		Type:        "day-suggestions",
		StartDate:   req.Date,
		EndDate:     req.Date,
		GeneratedAt: generatedAt,
		Insight:     sugType + "\n" + suggestions,
	})
	return c.JSON(http.StatusOK, map[string]any{"suggestions": suggestions, "type": sugType, "generated_at": generatedAt})
}

// GET /api/suggestions/day?date=...
func (h *Handler) GetStoredDaySuggestions(c *echo.Context) error {
	session := auth.SessionFrom(c)

	date := c.QueryParam("date")
	if date == "" {
		return writeErr(c, http.StatusBadRequest, "date required")
	}
	svc, err := h.sheetsSvc(c, session)
	if err != nil {
		return h.writeAPIErr(c, err)
	}
	rec, err := svc.GetInsight(c.Request().Context(), "day-suggestions", date, date)
	if err != nil {
		return h.writeAPIErr(c, err)
	}
	if rec == nil {
		return c.JSON(http.StatusOK, map[string]any{"suggestions": nil, "type": nil, "generated_at": nil})
	}
	sugType := "remaining"
	sugText := rec.Insight
	if parts := strings.SplitN(rec.Insight, "\n", 2); len(parts) == 2 {
		sugType = parts[0]
		sugText = parts[1]
	}
	return c.JSON(http.StatusOK, map[string]any{"suggestions": sugText, "type": sugType, "generated_at": rec.GeneratedAt})
}

// POST /api/suggestions/week
func (h *Handler) WeekSuggestions(c *echo.Context) error {
	session := auth.SessionFrom(c)

	r := c.Request()
	ctx := r.Context()
	var req struct {
		Start string `json:"start"`
		End   string `json:"end"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Start == "" || req.End == "" {
		return writeErr(c, http.StatusBadRequest, "start and end dates required")
	}
	if _, err := time.Parse("2006-01-02", req.Start); err != nil {
		return writeErr(c, http.StatusBadRequest, "invalid start date")
	}
	if _, err := time.Parse("2006-01-02", req.End); err != nil {
		return writeErr(c, http.StatusBadRequest, "invalid end date")
	}

	svc, err := h.sheetsSvc(c, session)
	if err != nil {
		return h.writeAPIErr(c, err)
	}

	entries, err := svc.GetFoodByDateRange(ctx, req.Start, req.End)
	if err != nil {
		return h.writeAPIErr(c, err)
	}
	dailyLogs, err := svc.GetActivityByDateRange(ctx, req.Start, req.End)
	if err != nil {
		return h.writeAPIErr(c, err)
	}

	summary := buildWeekSummary(req.Start, req.End, entries, dailyLogs)

	if rec, _ := svc.GetInsight(ctx, "week", req.Start, req.End); rec != nil {
		summary += "\nInsights for this week:\n" + rec.Insight + "\n"
	}

	profileCacheKey := session.SpreadsheetID + "|profile"
	var profileCtx string
	if cached, ok := h.cacheGet(profileCacheKey); ok {
		profileCtx = string(cached)
	} else {
		profile, _ := svc.GetProfile(ctx)
		profileCtx = formatProfileContext(profile, LocalNow(r).Year())
		h.cacheSet(profileCacheKey, []byte(profileCtx))
	}

	suggestions, err := h.gemini.WeekMealSuggestions(ctx, summary, profileCtx)
	if err != nil {
		return writeErr(c, http.StatusInternalServerError, "gemini error: "+err.Error())
	}
	generatedAt := time.Now().UTC().Format(time.RFC3339)
	_ = svc.SaveInsight(ctx, sheets.InsightRecord{
		Type:        "week-suggestions",
		StartDate:   req.Start,
		EndDate:     req.End,
		GeneratedAt: generatedAt,
		Insight:     suggestions,
	})
	return c.JSON(http.StatusOK, map[string]any{"suggestions": suggestions, "generated_at": generatedAt})
}

// GET /api/suggestions/week?start=...&end=...
func (h *Handler) GetStoredWeekSuggestions(c *echo.Context) error {
	session := auth.SessionFrom(c)

	start := c.QueryParam("start")
	end := c.QueryParam("end")
	if start == "" || end == "" {
		return writeErr(c, http.StatusBadRequest, "start and end required")
	}
	svc, err := h.sheetsSvc(c, session)
	if err != nil {
		return h.writeAPIErr(c, err)
	}
	rec, err := svc.GetInsight(c.Request().Context(), "week-suggestions", start, end)
	if err != nil {
		return h.writeAPIErr(c, err)
	}
	if rec == nil {
		return c.JSON(http.StatusOK, map[string]any{"suggestions": nil, "generated_at": nil})
	}
	return c.JSON(http.StatusOK, map[string]any{"suggestions": rec.Insight, "generated_at": rec.GeneratedAt})
}

// POST /api/insights/meal
// POST /api/suggestions/meal
func (h *Handler) MealSuggestion(c *echo.Context) error {
	session := auth.SessionFrom(c)

	r := c.Request()
	ctx := r.Context()
	var req struct {
		Date string `json:"date"`
		Meal string `json:"meal"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Date == "" || req.Meal == "" {
		return writeErr(c, http.StatusBadRequest, "date and meal required")
	}
	if _, err := time.Parse("2006-01-02", req.Date); err != nil {
		return writeErr(c, http.StatusBadRequest, "invalid date")
	}

	svc, err := h.sheetsSvc(c, session)
	if err != nil {
		return h.writeAPIErr(c, err)
	}

	profileCacheKey := session.SpreadsheetID + "|profile"
	var profileCtx string
	if cached, ok := h.cacheGet(profileCacheKey); ok {
		profileCtx = string(cached)
	} else {
		profile, _ := svc.GetProfile(ctx)
		profileCtx = formatProfileContext(profile, LocalNow(r).Year())
		h.cacheSet(profileCacheKey, []byte(profileCtx))
	}

	summary, err := h.buildSingleMealSuggestionSummary(ctx, svc, req.Date, req.Meal)
	if err != nil {
		return h.writeAPIErr(c, err)
	}

	insightType := "meal-suggestion:" + req.Meal

	suggestion, err := h.gemini.SingleMealSuggestion(ctx, summary, profileCtx)
	if err != nil {
		return writeErr(c, http.StatusInternalServerError, "gemini error: "+err.Error())
	}
	generatedAt := time.Now().UTC().Format(time.RFC3339)
	_ = svc.SaveInsight(ctx, sheets.InsightRecord{
		Type:        insightType,
		StartDate:   req.Date,
		EndDate:     req.Date,
		GeneratedAt: generatedAt,
		Insight:     suggestion,
	})
	return c.JSON(http.StatusOK, map[string]any{"suggestion": suggestion, "generated_at": generatedAt})
}

// GET /api/suggestions/meal?date=...&meal=...
func (h *Handler) GetStoredMealSuggestion(c *echo.Context) error {
	session := auth.SessionFrom(c)

	date := c.QueryParam("date")
	meal := c.QueryParam("meal")
	if date == "" || meal == "" {
		return writeErr(c, http.StatusBadRequest, "date and meal required")
	}
	svc, err := h.sheetsSvc(c, session)
	if err != nil {
		return h.writeAPIErr(c, err)
	}
	insightType := "meal-suggestion:" + meal
	rec, err := svc.GetInsight(c.Request().Context(), insightType, date, date)
	if err != nil {
		return h.writeAPIErr(c, err)
	}
	if rec == nil {
		return c.JSON(http.StatusOK, map[string]any{"suggestion": nil, "generated_at": nil})
	}
	return c.JSON(http.StatusOK, map[string]any{"suggestion": rec.Insight, "generated_at": rec.GeneratedAt})
}

func (h *Handler) buildSingleMealSuggestionSummary(ctx context.Context, svc *sheets.Service, date, mealType string) (string, error) {
	entries, err := svc.GetFoodByDateRange(ctx, date, date)
	if err != nil {
		return "", err
	}

	prevDate := addDaysStr(date, -1)
	prevEntries, _ := svc.GetFoodByDateRange(ctx, prevDate, prevDate)

	// Try to get today's day insight for context; fall back to yesterday's.
	var insightText string
	if rec, _ := svc.GetInsight(ctx, "day", date, date); rec != nil {
		insightText = rec.Insight
	} else if rec, _ := svc.GetInsight(ctx, "day", prevDate, prevDate); rec != nil {
		insightText = rec.Insight
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Suggest one meal for: %s\n\n", mealType)

	if insightText != "" {
		fmt.Fprintf(&b, "Nutrition insights (factor into suggestion):\n%s\n\n", insightText)
	}

	if len(entries) > 0 {
		fmt.Fprintf(&b, "Already eaten today:\n")
		totalCal, totalProt := 0, 0
		for _, e := range entries {
			fmt.Fprintf(&b, "  - [%s] %s: %d cal, %dg protein, %dg fiber\n", e.MealType, e.Description, e.Calories, e.Protein, e.Fiber)
			totalCal += e.Calories
			totalProt += e.Protein
		}
		fmt.Fprintf(&b, "  Today's totals so far: %d cal, %dg protein\n\n", totalCal, totalProt)
	}

	if len(prevEntries) > 0 {
		fmt.Fprintf(&b, "Yesterday's meals (avoid repeating):\n")
		for _, e := range prevEntries {
			fmt.Fprintf(&b, "  - [%s] %s\n", e.MealType, e.Description)
		}
	}

	return b.String(), nil
}

func addDaysStr(dateStr string, n int) string {
	t, _ := time.Parse("2006-01-02", dateStr)
	return t.AddDate(0, 0, n).Format("2006-01-02")
}

func buildWeekSummary(start, end string, entries []sheets.FoodEntry, dailyLogs []sheets.DayLog) string {
	byDate := map[string][]sheets.FoodEntry{}
	for _, e := range entries {
		byDate[e.Date] = append(byDate[e.Date], e)
	}
	logByDate := map[string]sheets.DayLog{}
	for _, l := range dailyLogs {
		logByDate[l.Date] = l
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Week %s to %s:\n\n", start, end)
	cur, _ := time.Parse("2006-01-02", start)
	endT, _ := time.Parse("2006-01-02", end)
	for !cur.After(endT) {
		date := cur.Format("2006-01-02")
		dayEntries := byDate[date]
		log := logByDate[date]
		fmt.Fprintf(&b, "%s (%s):\n", date, cur.Weekday())
		if len(dayEntries) == 0 {
			fmt.Fprintf(&b, "  No food logged\n")
		} else {
			totalCal, totalProt, totalCarb, totalFat, totalFiber := 0, 0, 0, 0, 0
			for _, e := range dayEntries {
				totalCal += e.Calories
				totalProt += e.Protein
				totalCarb += e.Carbs
				totalFat += e.Fat
				totalFiber += e.Fiber
			}
			fmt.Fprintf(&b, "  Totals: %d cal, %dg protein, %dg carbs, %dg fat, %dg fiber\n", totalCal, totalProt, totalCarb, totalFat, totalFiber)
			for _, e := range dayEntries {
				fmt.Fprintf(&b, "  - [%s] %s: %d cal\n", e.MealType, e.Description, e.Calories)
			}
		}
		if log.Activity != "" {
			fmt.Fprintf(&b, "  Activity: %s\n", log.Activity)
		}
		if log.Poop {
			fmt.Fprintf(&b, "  Bowel movement: yes\n")
			if log.PoopNotes != "" {
				fmt.Fprintf(&b, "  Notes: %s\n", log.PoopNotes)
			}
		}
		if log.Hydration > 0 {
			fmt.Fprintf(&b, "  Water: %.1fL\n", log.Hydration)
		}
		if log.FeelingScore > 0 {
			fmt.Fprintf(&b, "  Feeling: %d/10\n", log.FeelingScore)
		}
		fmt.Fprintln(&b)
		cur = cur.AddDate(0, 0, 1)
	}
	return b.String()
}

func buildDaySummary(date string, entries []sheets.FoodEntry, dailyLogs []sheets.DayLog, inProgress bool) string {
	logByDate := map[string]sheets.DayLog{}
	for _, l := range dailyLogs {
		logByDate[l.Date] = l
	}
	t, _ := time.Parse("2006-01-02", date)
	log := logByDate[date]

	var b strings.Builder
	fmt.Fprintf(&b, "Day: %s (%s)\n", date, t.Weekday())
	if inProgress {
		fmt.Fprintf(&b, "Status: TODAY — day is still in progress; more meals may be logged later.\n")
	} else {
		fmt.Fprintf(&b, "Status: past day — complete log.\n")
	}
	b.WriteString("\n")
	if len(entries) == 0 {
		fmt.Fprintf(&b, "  No food logged\n")
	} else {
		totalCal, totalProt, totalCarb, totalFat, totalFiber := 0, 0, 0, 0, 0
		for _, e := range entries {
			totalCal += e.Calories
			totalProt += e.Protein
			totalCarb += e.Carbs
			totalFat += e.Fat
			totalFiber += e.Fiber
		}
		fmt.Fprintf(&b, "Totals: %d cal, %dg protein, %dg carbs, %dg fat, %dg fiber\n", totalCal, totalProt, totalCarb, totalFat, totalFiber)
		for _, e := range entries {
			fmt.Fprintf(&b, "  - [%s] %s: %d cal\n", e.MealType, e.Description, e.Calories)
		}
	}
	if log.Activity != "" {
		fmt.Fprintf(&b, "Activity: %s\n", log.Activity)
	}
	if log.Poop {
		fmt.Fprintf(&b, "Bowel movement: yes\n")
		if log.PoopNotes != "" {
			fmt.Fprintf(&b, "Notes: %s\n", log.PoopNotes)
		}
	}
	if log.Hydration > 0 {
		fmt.Fprintf(&b, "Water: %.1fL\n", log.Hydration)
	}
	if log.FeelingScore > 0 {
		fmt.Fprintf(&b, "Feeling: %d/10\n", log.FeelingScore)
		if log.FeelingNotes != "" {
			fmt.Fprintf(&b, "Feeling notes: %s\n", log.FeelingNotes)
		}
	}
	return b.String()
}

func buildMealSuggestionSummary(date string, entries, prevEntries []sheets.FoodEntry, complete bool, insightText string) string {
	var b strings.Builder

	if complete {
		fmt.Fprintf(&b, "Today (%s) is complete. Suggest meals for tomorrow.\n\n", date)
	} else {
		hasMeal := map[string]bool{}
		for _, e := range entries {
			hasMeal[e.MealType] = true
		}
		var missing []string
		for _, m := range []string{"breakfast", "lunch", "dinner"} {
			if !hasMeal[m] {
				missing = append(missing, m)
			}
		}
		fmt.Fprintf(&b, "Suggest meals for: %s\n\n", strings.Join(missing, ", "))
	}

	if insightText != "" {
		fmt.Fprintf(&b, "Nutrition insights for today (factor these into suggestions):\n%s\n\n", insightText)
	}

	if len(entries) > 0 {
		fmt.Fprintf(&b, "Already eaten today:\n")
		for _, e := range entries {
			fmt.Fprintf(&b, "  - [%s] %s: %d cal, %dg protein\n", e.MealType, e.Description, e.Calories, e.Protein)
		}
		totalCal, totalProt := 0, 0
		for _, e := range entries {
			totalCal += e.Calories
			totalProt += e.Protein
		}
		fmt.Fprintf(&b, "  Today's totals so far: %d cal, %dg protein\n\n", totalCal, totalProt)
	}

	if len(prevEntries) > 0 {
		fmt.Fprintf(&b, "Yesterday's meals (avoid repeating):\n")
		for _, e := range prevEntries {
			fmt.Fprintf(&b, "  - [%s] %s\n", e.MealType, e.Description)
		}
	}

	return b.String()
}
