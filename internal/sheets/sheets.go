package sheets

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/oauth2"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	googlesheets "google.golang.org/api/sheets/v4"
)

const (
	foodSheet      = "Food"
	activitySheet  = "Activity"
	eventsSheet    = "Events"
	metaSheet      = "Meta"
	profileSheet   = "Profile"
	insightsSheet  = "Insights"
	favoritesSheet = "Favorites"

	CurrentSchemaVersion = 12
)

// Event kinds. Each maps to a row in the Events sheet.
const (
	EventKindWorkout = "workout"
	EventKindStool   = "stool"
	EventKindWater   = "water"
	EventKindFeeling = "feeling"
)

// Event is one row in the Events sheet — a timestamped non-meal entry on the
// day timeline. Schema: id | date | time | kind | text | num | notes
//   - workout : text=description,           num=optional duration_min
//   - stool   : text=optional description,  num=unused
//   - water   : text=unused,                num=millilitres
//   - feeling : text=mood notes,            num=score 1-10 (0 = unset)
type Event struct {
	ID    string  `json:"id"`
	Date  string  `json:"date"`
	Time  string  `json:"time"`
	Kind  string  `json:"kind"`
	Text  string  `json:"text"`
	Num   float64 `json:"num"`
	Notes string  `json:"notes"`
}

func (e Event) ToRow() []interface{} {
	return []interface{}{
		e.ID, e.Date, e.Time, e.Kind, e.Text,
		strconv.FormatFloat(e.Num, 'f', -1, 64), e.Notes,
	}
}

func EventFromRow(row []interface{}) (*Event, error) {
	if len(row) < 4 {
		return nil, fmt.Errorf("event row has %d columns, need at least 4", len(row))
	}
	str := func(i int) string {
		if i >= len(row) {
			return ""
		}
		return fmt.Sprintf("%v", row[i])
	}
	fnum := func(i int) float64 {
		f, _ := strconv.ParseFloat(str(i), 64)
		return f
	}
	return &Event{
		ID: str(0), Date: str(1), Time: str(2), Kind: str(3),
		Text: str(4), Num: fnum(5), Notes: str(6),
	}, nil
}

// FoodEntry is one row in the Food sheet.
type FoodEntry struct {
	ID          string `json:"id"`
	Date        string `json:"date"`
	Time        string `json:"time"`
	MealType    string `json:"meal_type"`
	Description string `json:"description"`
	Calories    int    `json:"calories"`
	Protein     int    `json:"protein"`
	Carbs       int    `json:"carbs"`
	Fat         int    `json:"fat"`
	Fiber       int    `json:"fiber"`
}

func (e FoodEntry) ToRow() []interface{} {
	return []interface{}{
		e.ID, e.Date, e.Time, e.MealType, e.Description,
		strconv.Itoa(e.Calories), strconv.Itoa(e.Protein),
		strconv.Itoa(e.Carbs), strconv.Itoa(e.Fat), strconv.Itoa(e.Fiber),
	}
}

func FoodEntryFromRow(row []interface{}) (*FoodEntry, error) {
	if len(row) < 9 {
		return nil, fmt.Errorf("row has %d columns, need at least 9", len(row))
	}
	str := func(v interface{}) string { return fmt.Sprintf("%v", v) }
	num := func(v interface{}) int {
		n, _ := strconv.Atoi(fmt.Sprintf("%v", v))
		return n
	}
	e := &FoodEntry{
		ID: str(row[0]), Date: str(row[1]), Time: str(row[2]),
		MealType: str(row[3]), Description: str(row[4]),
		Calories: num(row[5]), Protein: num(row[6]),
		Carbs: num(row[7]), Fat: num(row[8]),
	}
	if len(row) >= 10 {
		e.Fiber = num(row[9])
	}
	return e, nil
}

func DateString(t time.Time) string { return t.Format("2006-01-02") }
func TimeString(t time.Time) string { return t.Format("15:04") }

// DayLog is one row in the Activity sheet.
// Schema: date | activity | feeling_score | feeling_notes | poop | poop_notes | hydration
// Backward compat: old 2-column rows (date | notes) map notes → activity.
type DayLog struct {
	Date         string  `json:"date"`
	Activity     string  `json:"activity"`
	FeelingScore int     `json:"feeling_score"` // 0 = not set, 1–10
	FeelingNotes string  `json:"feeling_notes"`
	Poop         bool    `json:"poop"`
	PoopNotes    string  `json:"poop_notes"`
	Hydration    float64 `json:"hydration"` // litres, 0 = not set
}

func (d DayLog) ToRow() []interface{} {
	return []interface{}{
		d.Date, d.Activity, strconv.Itoa(d.FeelingScore), d.FeelingNotes,
		strconv.FormatBool(d.Poop), d.PoopNotes, strconv.FormatFloat(d.Hydration, 'f', -1, 64),
	}
}

func DayLogFromRow(row []interface{}) DayLog {
	str := func(v interface{}) string { return fmt.Sprintf("%v", v) }
	num := func(v interface{}) int {
		n, _ := strconv.Atoi(fmt.Sprintf("%v", v))
		return n
	}
	fnum := func(v interface{}) float64 {
		f, _ := strconv.ParseFloat(fmt.Sprintf("%v", v), 64)
		return f
	}
	d := DayLog{}
	if len(row) >= 1 {
		d.Date = str(row[0])
	}
	if len(row) >= 2 {
		d.Activity = str(row[1])
	}
	if len(row) >= 3 {
		d.FeelingScore = num(row[2])
	}
	if len(row) >= 4 {
		d.FeelingNotes = str(row[3])
	}
	if len(row) >= 5 {
		d.Poop = str(row[4]) == "true"
	}
	if len(row) >= 6 {
		d.PoopNotes = str(row[5])
	}
	if len(row) >= 7 {
		d.Hydration = fnum(row[6])
	}
	return d
}

// UserProfile stores user context for improving Gemini macro estimates.
// Stored in the Profile sheet as a single data row: gender | height | weight | notes | goals | dietary_restrictions | birth_year | nutrition_expertise
type UserProfile struct {
	Gender              string `json:"gender"`
	Height              string `json:"height"`
	Weight              string `json:"weight"`
	Notes               string `json:"notes"`
	Goals               string `json:"goals"`
	DietaryRestrictions string `json:"dietary_restrictions"`
	BirthYear           string `json:"birth_year"`
	NutritionExpertise  string `json:"nutrition_expertise"`
}

func (p UserProfile) ToRow() []interface{} {
	return []interface{}{p.Gender, p.Height, p.Weight, p.Notes, p.Goals, p.DietaryRestrictions, p.BirthYear, p.NutritionExpertise}
}

func UserProfileFromRow(row []interface{}) UserProfile {
	str := func(i int) string {
		if i < len(row) {
			return fmt.Sprintf("%v", row[i])
		}
		return ""
	}
	return UserProfile{
		Gender:              str(0),
		Height:              str(1),
		Weight:              str(2),
		Notes:               str(3),
		Goals:               str(4),
		DietaryRestrictions: str(5),
		BirthYear:           str(6),
		NutritionExpertise:  str(7),
	}
}

// InsightRecord stores a generated AI insight in the Insights sheet.
// Schema: type | start_date | end_date | generated_at | insight | triggered_by
type InsightRecord struct {
	Type        string `json:"type"` // "day" or "week"
	StartDate   string `json:"start_date"`
	EndDate     string `json:"end_date"`
	GeneratedAt string `json:"generated_at"` // UTC RFC3339
	Insight     string `json:"insight"`
	TriggeredBy string `json:"triggered_by,omitempty"` // food entry ID that anchored this snapshot (day insights only)
}

// FavoriteEntry is one row in the Favorites sheet.
// Schema: id | description | meal_type | calories | protein | carbs | fat | fiber | created_at
type FavoriteEntry struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	MealType    string `json:"meal_type"`
	Calories    int    `json:"calories"`
	Protein     int    `json:"protein"`
	Carbs       int    `json:"carbs"`
	Fat         int    `json:"fat"`
	Fiber       int    `json:"fiber"`
	CreatedAt   string `json:"created_at"`
}

func (f FavoriteEntry) ToRow() []interface{} {
	return []interface{}{
		f.ID, f.Description, f.MealType,
		strconv.Itoa(f.Calories), strconv.Itoa(f.Protein),
		strconv.Itoa(f.Carbs), strconv.Itoa(f.Fat), strconv.Itoa(f.Fiber),
		f.CreatedAt,
	}
}

func FavoriteEntryFromRow(row []interface{}) (*FavoriteEntry, error) {
	if len(row) < 8 {
		return nil, fmt.Errorf("favorite row has %d columns, need at least 8", len(row))
	}
	str := func(v interface{}) string { return fmt.Sprintf("%v", v) }
	num := func(v interface{}) int {
		n, _ := strconv.Atoi(fmt.Sprintf("%v", v))
		return n
	}
	f := &FavoriteEntry{
		ID: str(row[0]), Description: str(row[1]), MealType: str(row[2]),
		Calories: num(row[3]), Protein: num(row[4]),
		Carbs: num(row[5]), Fat: num(row[6]), Fiber: num(row[7]),
	}
	if len(row) >= 9 {
		f.CreatedAt = str(row[8])
	}
	return f, nil
}

// GetFavorites returns all saved favorite entries.
func (s *Service) GetFavorites(ctx context.Context) ([]FavoriteEntry, error) {
	resp, err := s.svc.Spreadsheets.Values.Get(s.spreadsheetID, favoritesSheet+"!A:I").Context(ctx).Do()
	if err != nil {
		var ge *googleapi.Error
		if errors.As(err, &ge) && ge.Code == 400 {
			return nil, nil // sheet not yet created
		}
		return nil, err
	}
	var out []FavoriteEntry
	for i, row := range resp.Values {
		if i == 0 || len(row) < 8 {
			continue
		}
		f, err := FavoriteEntryFromRow(row)
		if err != nil {
			continue
		}
		out = append(out, *f)
	}
	return out, nil
}

// NormalizeFavoriteKey returns a canonical lookup key for a favorite
// description: lowercased, trimmed, with internal whitespace collapsed to a
// single space. Used to detect duplicates regardless of casing/spacing.
func NormalizeFavoriteKey(desc string) string {
	return strings.ToLower(strings.Join(strings.Fields(desc), " "))
}

// AddFavorite appends a favorite entry row.
func (s *Service) AddFavorite(ctx context.Context, f FavoriteEntry) error {
	vr := &googlesheets.ValueRange{Values: [][]interface{}{f.ToRow()}}
	_, err := s.svc.Spreadsheets.Values.Append(
		s.spreadsheetID, favoritesSheet+"!A:I", vr,
	).ValueInputOption("RAW").Context(ctx).Do()
	return err
}

// DeleteFavorite removes the favorite entry row with the given ID.
func (s *Service) DeleteFavorite(ctx context.Context, id string) error {
	ss, err := s.svc.Spreadsheets.Get(s.spreadsheetID).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("get spreadsheet: %w", err)
	}
	var sheetID int64 = -1
	for _, sh := range ss.Sheets {
		if sh.Properties.Title == favoritesSheet {
			sheetID = sh.Properties.SheetId
			break
		}
	}
	if sheetID < 0 {
		return fmt.Errorf("favorites sheet not found")
	}

	resp, err := s.svc.Spreadsheets.Values.Get(s.spreadsheetID, favoritesSheet+"!A:A").Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("get ids: %w", err)
	}
	rowIdx := -1
	for i, row := range resp.Values {
		if i == 0 {
			continue
		}
		if len(row) > 0 && fmt.Sprintf("%v", row[0]) == id {
			rowIdx = i
			break
		}
	}
	if rowIdx < 0 {
		return fmt.Errorf("favorite %q not found", id)
	}

	req := &googlesheets.BatchUpdateSpreadsheetRequest{
		Requests: []*googlesheets.Request{{
			DeleteDimension: &googlesheets.DeleteDimensionRequest{
				Range: &googlesheets.DimensionRange{
					SheetId:    sheetID,
					Dimension:  "ROWS",
					StartIndex: int64(rowIdx),
					EndIndex:   int64(rowIdx + 1),
				},
			},
		}},
	}
	_, err = s.svc.Spreadsheets.BatchUpdate(s.spreadsheetID, req).Context(ctx).Do()
	return err
}

// SaveInsight appends an insight record to the Insights sheet.
func (s *Service) SaveInsight(ctx context.Context, rec InsightRecord) error {
	vr := &googlesheets.ValueRange{
		Values: [][]interface{}{{rec.Type, rec.StartDate, rec.EndDate, rec.GeneratedAt, rec.Insight, rec.TriggeredBy}},
	}
	_, err := s.svc.Spreadsheets.Values.Append(
		s.spreadsheetID, insightsSheet+"!A:F", vr,
	).ValueInputOption("RAW").Context(ctx).Do()
	return err
}

func insightFromRow(row []interface{}) InsightRecord {
	str := func(v interface{}) string { return fmt.Sprintf("%v", v) }
	rec := InsightRecord{}
	if len(row) >= 1 {
		rec.Type = str(row[0])
	}
	if len(row) >= 2 {
		rec.StartDate = str(row[1])
	}
	if len(row) >= 3 {
		rec.EndDate = str(row[2])
	}
	if len(row) >= 4 {
		rec.GeneratedAt = str(row[3])
	}
	if len(row) >= 5 {
		rec.Insight = str(row[4])
	}
	if len(row) >= 6 {
		rec.TriggeredBy = str(row[5])
	}
	return rec
}

// GetInsight returns the most recently generated insight matching type+start+end,
// or nil if none exists. Returns nil (no error) when the Insights sheet does not exist yet.
func (s *Service) GetInsight(ctx context.Context, insightType, startDate, endDate string) (*InsightRecord, error) {
	resp, err := s.svc.Spreadsheets.Values.Get(s.spreadsheetID, insightsSheet+"!A:F").Context(ctx).Do()
	if err != nil {
		var ge *googleapi.Error
		if errors.As(err, &ge) && ge.Code == 400 {
			return nil, nil // sheet not yet created
		}
		return nil, err
	}
	var latest *InsightRecord
	for i, row := range resp.Values {
		if i == 0 || len(row) < 5 {
			continue
		}
		str := func(v interface{}) string { return fmt.Sprintf("%v", v) }
		if str(row[0]) == insightType && str(row[1]) == startDate && str(row[2]) == endDate {
			rec := insightFromRow(row)
			latest = &rec
		}
	}
	return latest, nil
}

// GetInsightSnapshotsByDate returns all day-type insight rows for the given date,
// in order of generation. Used to discover which entries anchor snapshots so the
// timeline can show per-meal bubbles. Returns nil (no error) if the sheet doesn't
// exist yet.
func (s *Service) GetInsightSnapshotsByDate(ctx context.Context, date string) ([]InsightRecord, error) {
	resp, err := s.svc.Spreadsheets.Values.Get(s.spreadsheetID, insightsSheet+"!A:F").Context(ctx).Do()
	if err != nil {
		var ge *googleapi.Error
		if errors.As(err, &ge) && ge.Code == 400 {
			return nil, nil
		}
		return nil, err
	}
	out := []InsightRecord{}
	for i, row := range resp.Values {
		if i == 0 || len(row) < 5 {
			continue
		}
		str := func(v interface{}) string { return fmt.Sprintf("%v", v) }
		if str(row[0]) != "day" || str(row[1]) != date || str(row[2]) != date {
			continue
		}
		out = append(out, insightFromRow(row))
	}
	return out, nil
}

// GetInsightByTrigger returns the insight anchored to the given food entry ID.
// Used to display per-meal "verdict at the time" insights on the timeline.
// Returns nil (no error) if no anchor matches or the sheet doesn't exist yet.
func (s *Service) GetInsightByTrigger(ctx context.Context, triggerEntryID string) (*InsightRecord, error) {
	if triggerEntryID == "" {
		return nil, nil
	}
	resp, err := s.svc.Spreadsheets.Values.Get(s.spreadsheetID, insightsSheet+"!A:F").Context(ctx).Do()
	if err != nil {
		var ge *googleapi.Error
		if errors.As(err, &ge) && ge.Code == 400 {
			return nil, nil
		}
		return nil, err
	}
	var latest *InsightRecord
	for i, row := range resp.Values {
		if i == 0 || len(row) < 6 {
			continue
		}
		str := func(v interface{}) string { return fmt.Sprintf("%v", v) }
		if str(row[5]) == triggerEntryID {
			rec := insightFromRow(row)
			latest = &rec
		}
	}
	return latest, nil
}

// GetProfile reads the user profile from the Profile sheet.
// Returns an empty UserProfile if no data has been saved yet.
func (s *Service) GetProfile(ctx context.Context) (UserProfile, error) {
	resp, err := s.svc.Spreadsheets.Values.Get(s.spreadsheetID, profileSheet+"!A2:H2").Context(ctx).Do()
	if err != nil {
		return UserProfile{}, fmt.Errorf("get profile: %w", err)
	}
	if len(resp.Values) == 0 {
		return UserProfile{}, nil
	}
	return UserProfileFromRow(resp.Values[0]), nil
}

// SetProfile writes the user profile to the Profile sheet (row 2).
func (s *Service) SetProfile(ctx context.Context, p UserProfile) error {
	vr := &googlesheets.ValueRange{Values: [][]interface{}{p.ToRow()}}
	_, err := s.svc.Spreadsheets.Values.Update(
		s.spreadsheetID, profileSheet+"!A2:H2", vr,
	).ValueInputOption("RAW").Context(ctx).Do()
	return err
}

// Service wraps the Sheets API scoped to one user's spreadsheet.
type Service struct {
	svc           *googlesheets.Service
	spreadsheetID string
}

func NewService(ctx context.Context, ts oauth2.TokenSource, spreadsheetID string) (*Service, error) {
	svc, err := googlesheets.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return nil, err
	}
	return &Service{svc: svc, spreadsheetID: spreadsheetID}, nil
}

// CreateSpreadsheet creates a new spreadsheet in the user's Drive.
func CreateSpreadsheet(ctx context.Context, ts oauth2.TokenSource, userEmail string) (string, error) {
	// Validate drive.file scope is present
	driveSvc, err := drive.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return "", fmt.Errorf("drive client: %w", err)
	}
	_ = driveSvc

	sheetsSvc, err := googlesheets.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return "", fmt.Errorf("sheets client: %w", err)
	}

	ss := &googlesheets.Spreadsheet{
		Properties: &googlesheets.SpreadsheetProperties{
			Title: fmt.Sprintf("Food Tracker — %s", userEmail),
		},
		Sheets: []*googlesheets.Sheet{
			{Properties: &googlesheets.SheetProperties{Title: foodSheet}},
			{Properties: &googlesheets.SheetProperties{Title: activitySheet}},
			{Properties: &googlesheets.SheetProperties{Title: eventsSheet}},
			{Properties: &googlesheets.SheetProperties{Title: metaSheet}},
			{Properties: &googlesheets.SheetProperties{Title: profileSheet}},
			{Properties: &googlesheets.SheetProperties{Title: insightsSheet}},
			{Properties: &googlesheets.SheetProperties{Title: favoritesSheet}},
		},
	}
	created, err := sheetsSvc.Spreadsheets.Create(ss).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("create spreadsheet: %w", err)
	}

	// Write header rows
	foodHeaders := &googlesheets.ValueRange{
		Values: [][]interface{}{{"id", "date", "time", "meal_type", "description", "calories", "protein", "carbs", "fat", "fiber"}},
	}
	_, err = sheetsSvc.Spreadsheets.Values.Update(
		created.SpreadsheetId, foodSheet+"!A1:J1", foodHeaders,
	).ValueInputOption("RAW").Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("food headers: %w", err)
	}

	actHeaders := &googlesheets.ValueRange{
		Values: [][]interface{}{{"date", "activity", "feeling_score", "feeling_notes", "poop", "poop_notes", "hydration"}},
	}
	_, err = sheetsSvc.Spreadsheets.Values.Update(
		created.SpreadsheetId, activitySheet+"!A1:G1", actHeaders,
	).ValueInputOption("RAW").Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("activity headers: %w", err)
	}

	eventHeaders := &googlesheets.ValueRange{
		Values: [][]interface{}{{"id", "date", "time", "kind", "text", "num", "notes"}},
	}
	_, err = sheetsSvc.Spreadsheets.Values.Update(
		created.SpreadsheetId, eventsSheet+"!A1:G1", eventHeaders,
	).ValueInputOption("RAW").Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("events headers: %w", err)
	}

	// Meta sheet: A1 = header "schema_version", A2 = value
	metaData := &googlesheets.ValueRange{
		Values: [][]interface{}{{"schema_version"}, {strconv.Itoa(CurrentSchemaVersion)}},
	}
	_, err = sheetsSvc.Spreadsheets.Values.Update(
		created.SpreadsheetId, metaSheet+"!A1:A2", metaData,
	).ValueInputOption("RAW").Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("meta init: %w", err)
	}

	// Profile sheet: headers row
	profHeaders := &googlesheets.ValueRange{
		Values: [][]interface{}{{"gender", "height", "weight", "notes", "goals", "dietary_restrictions", "birth_year"}},
	}
	_, err = sheetsSvc.Spreadsheets.Values.Update(
		created.SpreadsheetId, profileSheet+"!A1:G1", profHeaders,
	).ValueInputOption("RAW").Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("profile headers: %w", err)
	}

	// Insights sheet: headers row
	insightHeaders := &googlesheets.ValueRange{
		Values: [][]interface{}{{"type", "start_date", "end_date", "generated_at", "insight", "triggered_by"}},
	}
	_, err = sheetsSvc.Spreadsheets.Values.Update(
		created.SpreadsheetId, insightsSheet+"!A1:F1", insightHeaders,
	).ValueInputOption("RAW").Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("insights headers: %w", err)
	}

	// Favorites sheet: headers row
	favHeaders := &googlesheets.ValueRange{
		Values: [][]interface{}{{"id", "description", "meal_type", "calories", "protein", "carbs", "fat", "fiber", "created_at"}},
	}
	_, err = sheetsSvc.Spreadsheets.Values.Update(
		created.SpreadsheetId, favoritesSheet+"!A1:I1", favHeaders,
	).ValueInputOption("RAW").Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("favorites headers: %w", err)
	}

	return created.SpreadsheetId, nil
}

// MigrateV1toV2 upgrades an existing spreadsheet from schema v1 to v2.
// It extends the Activity sheet header to include poop and poop_notes columns.
func MigrateV1toV2(ctx context.Context, ts oauth2.TokenSource, spreadsheetID string) error {
	sheetsSvc, err := googlesheets.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return fmt.Errorf("sheets client: %w", err)
	}

	actHeaders := &googlesheets.ValueRange{
		Values: [][]interface{}{{"date", "activity", "feeling_score", "feeling_notes", "poop", "poop_notes"}},
	}
	_, err = sheetsSvc.Spreadsheets.Values.Update(
		spreadsheetID, activitySheet+"!A1:F1", actHeaders,
	).ValueInputOption("RAW").Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("migrate v1→v2 activity header: %w", err)
	}

	metaData := &googlesheets.ValueRange{Values: [][]interface{}{{"2"}}}
	_, err = sheetsSvc.Spreadsheets.Values.Update(
		spreadsheetID, metaSheet+"!A2", metaData,
	).ValueInputOption("RAW").Context(ctx).Do()
	return err
}

// MigrateV2toV3 upgrades an existing spreadsheet from schema v2 to v3.
// It adds the hydration column to the Activity sheet header.
func MigrateV2toV3(ctx context.Context, ts oauth2.TokenSource, spreadsheetID string) error {
	sheetsSvc, err := googlesheets.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return fmt.Errorf("sheets client: %w", err)
	}

	actHeaders := &googlesheets.ValueRange{
		Values: [][]interface{}{{"date", "activity", "feeling_score", "feeling_notes", "poop", "poop_notes", "hydration"}},
	}
	_, err = sheetsSvc.Spreadsheets.Values.Update(
		spreadsheetID, activitySheet+"!A1:G1", actHeaders,
	).ValueInputOption("RAW").Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("migrate v2→v3 activity header: %w", err)
	}

	metaData := &googlesheets.ValueRange{Values: [][]interface{}{{"3"}}}
	_, err = sheetsSvc.Spreadsheets.Values.Update(
		spreadsheetID, metaSheet+"!A2", metaData,
	).ValueInputOption("RAW").Context(ctx).Do()
	return err
}

// MigrateV3toV4 upgrades an existing spreadsheet from schema v3 to v4.
// It adds the goals column to the Profile sheet header.
func MigrateV3toV4(ctx context.Context, ts oauth2.TokenSource, spreadsheetID string) error {
	sheetsSvc, err := googlesheets.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return fmt.Errorf("sheets client: %w", err)
	}

	profHeaders := &googlesheets.ValueRange{
		Values: [][]interface{}{{"gender", "height", "weight", "notes", "goals"}},
	}
	_, err = sheetsSvc.Spreadsheets.Values.Update(
		spreadsheetID, profileSheet+"!A1:E1", profHeaders,
	).ValueInputOption("RAW").Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("migrate v3→v4 profile header: %w", err)
	}

	metaData := &googlesheets.ValueRange{Values: [][]interface{}{{"4"}}}
	_, err = sheetsSvc.Spreadsheets.Values.Update(
		spreadsheetID, metaSheet+"!A2", metaData,
	).ValueInputOption("RAW").Context(ctx).Do()
	return err
}

// MigrateV4toV5 upgrades an existing spreadsheet from schema v4 to v5.
// It adds the dietary_restrictions column to the Profile sheet header.
func MigrateV4toV5(ctx context.Context, ts oauth2.TokenSource, spreadsheetID string) error {
	sheetsSvc, err := googlesheets.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return fmt.Errorf("sheets client: %w", err)
	}

	profHeaders := &googlesheets.ValueRange{
		Values: [][]interface{}{{"gender", "height", "weight", "notes", "goals", "dietary_restrictions"}},
	}
	_, err = sheetsSvc.Spreadsheets.Values.Update(
		spreadsheetID, profileSheet+"!A1:F1", profHeaders,
	).ValueInputOption("RAW").Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("migrate v4→v5 profile header: %w", err)
	}

	metaData := &googlesheets.ValueRange{Values: [][]interface{}{{"5"}}}
	_, err = sheetsSvc.Spreadsheets.Values.Update(
		spreadsheetID, metaSheet+"!A2", metaData,
	).ValueInputOption("RAW").Context(ctx).Do()
	return err
}

// MigrateV5toV6 upgrades an existing spreadsheet from schema v5 to v6.
// It adds the Insights sheet for persisting generated AI insights.
func MigrateV5toV6(ctx context.Context, ts oauth2.TokenSource, spreadsheetID string) error {
	sheetsSvc, err := googlesheets.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return fmt.Errorf("sheets client: %w", err)
	}

	// Add Insights sheet; ignore error if it already exists.
	_, _ = sheetsSvc.Spreadsheets.BatchUpdate(spreadsheetID, &googlesheets.BatchUpdateSpreadsheetRequest{
		Requests: []*googlesheets.Request{{
			AddSheet: &googlesheets.AddSheetRequest{
				Properties: &googlesheets.SheetProperties{Title: insightsSheet},
			},
		}},
	}).Context(ctx).Do()

	insightHeaders := &googlesheets.ValueRange{
		Values: [][]interface{}{{"type", "start_date", "end_date", "generated_at", "insight"}},
	}
	_, err = sheetsSvc.Spreadsheets.Values.Update(
		spreadsheetID, insightsSheet+"!A1:E1", insightHeaders,
	).ValueInputOption("RAW").Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("migrate v5→v6 insights header: %w", err)
	}

	metaData := &googlesheets.ValueRange{Values: [][]interface{}{{"6"}}}
	_, err = sheetsSvc.Spreadsheets.Values.Update(
		spreadsheetID, metaSheet+"!A2", metaData,
	).ValueInputOption("RAW").Context(ctx).Do()
	return err
}

// MigrateV6toV7 upgrades an existing spreadsheet from schema v6 to v7.
// It adds the age column to the Profile sheet header.
func MigrateV6toV7(ctx context.Context, ts oauth2.TokenSource, spreadsheetID string) error {
	sheetsSvc, err := googlesheets.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return fmt.Errorf("sheets client: %w", err)
	}

	profHeaders := &googlesheets.ValueRange{
		Values: [][]interface{}{{"gender", "height", "weight", "notes", "goals", "dietary_restrictions", "age"}},
	}
	_, err = sheetsSvc.Spreadsheets.Values.Update(
		spreadsheetID, profileSheet+"!A1:G1", profHeaders,
	).ValueInputOption("RAW").Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("migrate v6→v7 profile header: %w", err)
	}

	metaData := &googlesheets.ValueRange{Values: [][]interface{}{{"7"}}}
	_, err = sheetsSvc.Spreadsheets.Values.Update(
		spreadsheetID, metaSheet+"!A2", metaData,
	).ValueInputOption("RAW").Context(ctx).Do()
	return err
}

// MigrateV7toV8 upgrades an existing spreadsheet from schema v7 to v8.
// It adds the Favorites sheet for storing saved food entries.
func MigrateV7toV8(ctx context.Context, ts oauth2.TokenSource, spreadsheetID string) error {
	sheetsSvc, err := googlesheets.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return fmt.Errorf("sheets client: %w", err)
	}

	// Add Favorites sheet; ignore error if it already exists.
	_, _ = sheetsSvc.Spreadsheets.BatchUpdate(spreadsheetID, &googlesheets.BatchUpdateSpreadsheetRequest{
		Requests: []*googlesheets.Request{{
			AddSheet: &googlesheets.AddSheetRequest{
				Properties: &googlesheets.SheetProperties{Title: favoritesSheet},
			},
		}},
	}).Context(ctx).Do()

	favHeaders := &googlesheets.ValueRange{
		Values: [][]interface{}{{"id", "description", "meal_type", "calories", "protein", "carbs", "fat", "fiber", "created_at"}},
	}
	_, err = sheetsSvc.Spreadsheets.Values.Update(
		spreadsheetID, favoritesSheet+"!A1:I1", favHeaders,
	).ValueInputOption("RAW").Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("migrate v7→v8 favorites header: %w", err)
	}

	metaData := &googlesheets.ValueRange{Values: [][]interface{}{{"8"}}}
	_, err = sheetsSvc.Spreadsheets.Values.Update(
		spreadsheetID, metaSheet+"!A2", metaData,
	).ValueInputOption("RAW").Context(ctx).Do()
	return err
}

// MigrateV8toV9 upgrades an existing spreadsheet from schema v8 to v9.
// Renames the Profile column "age" to "birth_year" and converts any existing
// integer age value into a birth year using the server's current year.
// Non-numeric or out-of-range values are cleared.
func MigrateV8toV9(ctx context.Context, ts oauth2.TokenSource, spreadsheetID string) error {
	sheetsSvc, err := googlesheets.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return fmt.Errorf("sheets client: %w", err)
	}

	// Read existing age cell (G2) before rewriting the header.
	existing, err := sheetsSvc.Spreadsheets.Values.Get(spreadsheetID, profileSheet+"!G2").Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("migrate v8→v9 read age: %w", err)
	}
	converted := ""
	if len(existing.Values) > 0 && len(existing.Values[0]) > 0 {
		raw := strings.TrimSpace(fmt.Sprintf("%v", existing.Values[0][0]))
		if n, convErr := strconv.Atoi(raw); convErr == nil && n > 0 && n < 130 {
			converted = strconv.Itoa(time.Now().Year() - n)
		}
	}

	profHeaders := &googlesheets.ValueRange{
		Values: [][]interface{}{{"gender", "height", "weight", "notes", "goals", "dietary_restrictions", "birth_year"}},
	}
	_, err = sheetsSvc.Spreadsheets.Values.Update(
		spreadsheetID, profileSheet+"!A1:G1", profHeaders,
	).ValueInputOption("RAW").Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("migrate v8→v9 profile header: %w", err)
	}

	// Overwrite G2 with the converted birth year (or clear it).
	ageCell := &googlesheets.ValueRange{Values: [][]interface{}{{converted}}}
	_, err = sheetsSvc.Spreadsheets.Values.Update(
		spreadsheetID, profileSheet+"!G2", ageCell,
	).ValueInputOption("RAW").Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("migrate v8→v9 profile value: %w", err)
	}

	metaData := &googlesheets.ValueRange{Values: [][]interface{}{{"9"}}}
	_, err = sheetsSvc.Spreadsheets.Values.Update(
		spreadsheetID, metaSheet+"!A2", metaData,
	).ValueInputOption("RAW").Context(ctx).Do()
	return err
}

// MigrateV9toV10 upgrades an existing spreadsheet from schema v9 to v10.
// Adds a "triggered_by" column to the Insights sheet so day-level insights
// can be anchored to the entry that triggered their generation.
func MigrateV9toV10(ctx context.Context, ts oauth2.TokenSource, spreadsheetID string) error {
	sheetsSvc, err := googlesheets.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return fmt.Errorf("sheets client: %w", err)
	}

	insightHeaders := &googlesheets.ValueRange{
		Values: [][]interface{}{{"type", "start_date", "end_date", "generated_at", "insight", "triggered_by"}},
	}
	_, err = sheetsSvc.Spreadsheets.Values.Update(
		spreadsheetID, insightsSheet+"!A1:F1", insightHeaders,
	).ValueInputOption("RAW").Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("migrate v9→v10 insights header: %w", err)
	}

	metaData := &googlesheets.ValueRange{Values: [][]interface{}{{"10"}}}
	_, err = sheetsSvc.Spreadsheets.Values.Update(
		spreadsheetID, metaSheet+"!A2", metaData,
	).ValueInputOption("RAW").Context(ctx).Do()
	return err
}

// MigrateV10toV11 adds the Events sheet and fans the per-day Activity rows
// out into individual events at 12:00. The Activity sheet is left in place so
// the migration is reversible if needed; nothing reads from it after v11.
func MigrateV10toV11(ctx context.Context, ts oauth2.TokenSource, spreadsheetID string) error {
	sheetsSvc, err := googlesheets.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return fmt.Errorf("sheets client: %w", err)
	}

	_, _ = sheetsSvc.Spreadsheets.BatchUpdate(spreadsheetID, &googlesheets.BatchUpdateSpreadsheetRequest{
		Requests: []*googlesheets.Request{{
			AddSheet: &googlesheets.AddSheetRequest{
				Properties: &googlesheets.SheetProperties{Title: eventsSheet},
			},
		}},
	}).Context(ctx).Do()

	eventHeaders := &googlesheets.ValueRange{
		Values: [][]interface{}{{"id", "date", "time", "kind", "text", "num", "notes"}},
	}
	_, err = sheetsSvc.Spreadsheets.Values.Update(
		spreadsheetID, eventsSheet+"!A1:G1", eventHeaders,
	).ValueInputOption("RAW").Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("migrate v10→v11 events header: %w", err)
	}

	// Skip the activity-fanout if events already exist — this migration is
	// being re-run after the schema bump. Idempotency guard added after a
	// crash-mid-migration produced doubled events for some users.
	existing, err := sheetsSvc.Spreadsheets.Values.Get(spreadsheetID, eventsSheet+"!A:G").Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("migrate v10→v11 read events: %w", err)
	}
	if len(existing.Values) > 1 {
		metaData := &googlesheets.ValueRange{Values: [][]interface{}{{"11"}}}
		_, err = sheetsSvc.Spreadsheets.Values.Update(
			spreadsheetID, metaSheet+"!A2", metaData,
		).ValueInputOption("RAW").Context(ctx).Do()
		return err
	}

	resp, err := sheetsSvc.Spreadsheets.Values.Get(spreadsheetID, activitySheet+"!A:G").Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("migrate v10→v11 read activity: %w", err)
	}

	const noon = "12:00"
	var rows [][]interface{}
	for i, row := range resp.Values {
		if i == 0 || len(row) < 1 {
			continue
		}
		dl := DayLogFromRow(row)
		if dl.Date == "" {
			continue
		}
		mk := func(kind, text string, num float64) []interface{} {
			return Event{
				ID: uuid.NewString(), Date: dl.Date, Time: noon,
				Kind: kind, Text: text, Num: num,
			}.ToRow()
		}
		if strings.TrimSpace(dl.Activity) != "" {
			rows = append(rows, mk(EventKindWorkout, dl.Activity, 0))
		}
		if dl.Poop {
			rows = append(rows, mk(EventKindStool, dl.PoopNotes, 0))
		}
		if dl.Hydration > 0 {
			rows = append(rows, mk(EventKindWater, "", dl.Hydration*1000))
		}
		if dl.FeelingScore > 0 || strings.TrimSpace(dl.FeelingNotes) != "" {
			rows = append(rows, mk(EventKindFeeling, dl.FeelingNotes, float64(dl.FeelingScore)))
		}
	}
	if len(rows) > 0 {
		_, err = sheetsSvc.Spreadsheets.Values.Append(
			spreadsheetID, eventsSheet+"!A:G",
			&googlesheets.ValueRange{Values: rows},
		).ValueInputOption("RAW").Context(ctx).Do()
		if err != nil {
			return fmt.Errorf("migrate v10→v11 append events: %w", err)
		}
	}

	metaData := &googlesheets.ValueRange{Values: [][]interface{}{{"11"}}}
	_, err = sheetsSvc.Spreadsheets.Values.Update(
		spreadsheetID, metaSheet+"!A2", metaData,
	).ValueInputOption("RAW").Context(ctx).Do()
	return err
}

// MigrateV11toV12 deduplicates the Events sheet. A bug in earlier MigrateV10toV11
// runs (no idempotency guard) caused some users to have every fanned-out activity
// event appended twice. This pass deletes any event rows whose
// date+time+kind+text+num signature matches a row above them, keeping the first.
func MigrateV11toV12(ctx context.Context, ts oauth2.TokenSource, spreadsheetID string) error {
	sheetsSvc, err := googlesheets.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return fmt.Errorf("sheets client: %w", err)
	}

	ss, err := sheetsSvc.Spreadsheets.Get(spreadsheetID).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("migrate v11→v12 get spreadsheet: %w", err)
	}
	var sheetID int64 = -1
	for _, sh := range ss.Sheets {
		if sh.Properties.Title == eventsSheet {
			sheetID = sh.Properties.SheetId
			break
		}
	}

	if sheetID >= 0 {
		resp, err := sheetsSvc.Spreadsheets.Values.Get(spreadsheetID, eventsSheet+"!A:G").Context(ctx).Do()
		if err != nil {
			return fmt.Errorf("migrate v11→v12 read events: %w", err)
		}
		seen := map[string]bool{}
		var dupRows []int // 0-based row indices to delete
		str := func(row []interface{}, i int) string {
			if i >= len(row) {
				return ""
			}
			return fmt.Sprintf("%v", row[i])
		}
		for i, row := range resp.Values {
			if i == 0 || len(row) < 4 {
				continue
			}
			sig := str(row, 1) + "|" + str(row, 2) + "|" + str(row, 3) + "|" + str(row, 4) + "|" + str(row, 5)
			if seen[sig] {
				dupRows = append(dupRows, i)
				continue
			}
			seen[sig] = true
		}
		// Delete bottom-up so earlier indices stay valid.
		if len(dupRows) > 0 {
			reqs := make([]*googlesheets.Request, 0, len(dupRows))
			for j := len(dupRows) - 1; j >= 0; j-- {
				idx := dupRows[j]
				reqs = append(reqs, &googlesheets.Request{
					DeleteDimension: &googlesheets.DeleteDimensionRequest{
						Range: &googlesheets.DimensionRange{
							SheetId:    sheetID,
							Dimension:  "ROWS",
							StartIndex: int64(idx),
							EndIndex:   int64(idx + 1),
						},
					},
				})
			}
			_, err := sheetsSvc.Spreadsheets.BatchUpdate(spreadsheetID, &googlesheets.BatchUpdateSpreadsheetRequest{Requests: reqs}).Context(ctx).Do()
			if err != nil {
				return fmt.Errorf("migrate v11→v12 dedupe: %w", err)
			}
		}
	}

	metaData := &googlesheets.ValueRange{Values: [][]interface{}{{"12"}}}
	_, err = sheetsSvc.Spreadsheets.Values.Update(
		spreadsheetID, metaSheet+"!A2", metaData,
	).ValueInputOption("RAW").Context(ctx).Do()
	return err
}

// MigrateSpreadsheet is an alias kept for backwards compatibility; calls MigrateV1toV2.
func MigrateSpreadsheet(ctx context.Context, ts oauth2.TokenSource, spreadsheetID string) error {
	return MigrateV1toV2(ctx, ts, spreadsheetID)
}

// AppendEvent appends an event row.
func (s *Service) AppendEvent(ctx context.Context, e Event) error {
	vr := &googlesheets.ValueRange{Values: [][]interface{}{e.ToRow()}}
	_, err := s.svc.Spreadsheets.Values.Append(
		s.spreadsheetID, eventsSheet+"!A:G", vr,
	).ValueInputOption("RAW").Context(ctx).Do()
	return err
}

// GetEventsByDate returns all events for a given date (YYYY-MM-DD).
func (s *Service) GetEventsByDate(ctx context.Context, date string) ([]Event, error) {
	return s.getEventsFiltered(ctx, func(d string) bool { return d == date })
}

// GetEventsByDateRange returns events where start <= date <= end.
func (s *Service) GetEventsByDateRange(ctx context.Context, start, end string) ([]Event, error) {
	return s.getEventsFiltered(ctx, func(d string) bool { return d >= start && d <= end })
}

func (s *Service) getEventsFiltered(ctx context.Context, keep func(string) bool) ([]Event, error) {
	resp, err := s.svc.Spreadsheets.Values.Get(s.spreadsheetID, eventsSheet+"!A:G").Context(ctx).Do()
	if err != nil {
		var ge *googleapi.Error
		if errors.As(err, &ge) && ge.Code == 400 {
			return nil, nil
		}
		return nil, err
	}
	var out []Event
	for i, row := range resp.Values {
		if i == 0 || len(row) < 4 {
			continue
		}
		if !keep(fmt.Sprintf("%v", row[1])) {
			continue
		}
		e, err := EventFromRow(row)
		if err != nil {
			continue
		}
		out = append(out, *e)
	}
	return out, nil
}

// UpdateEvent replaces the event row with the given ID.
func (s *Service) UpdateEvent(ctx context.Context, id string, updated Event) error {
	resp, err := s.svc.Spreadsheets.Values.Get(s.spreadsheetID, eventsSheet+"!A:A").Context(ctx).Do()
	if err != nil {
		return err
	}
	rowNum := -1
	for i, row := range resp.Values {
		if i == 0 {
			continue
		}
		if len(row) > 0 && fmt.Sprintf("%v", row[0]) == id {
			rowNum = i + 1
			break
		}
	}
	if rowNum < 0 {
		return fmt.Errorf("event %q not found", id)
	}
	vr := &googlesheets.ValueRange{Values: [][]interface{}{updated.ToRow()}}
	_, err = s.svc.Spreadsheets.Values.Update(
		s.spreadsheetID,
		fmt.Sprintf("%s!A%d:G%d", eventsSheet, rowNum, rowNum),
		vr,
	).ValueInputOption("RAW").Context(ctx).Do()
	return err
}

// DeleteEvent removes the event row with the given ID.
func (s *Service) DeleteEvent(ctx context.Context, id string) error {
	ss, err := s.svc.Spreadsheets.Get(s.spreadsheetID).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("get spreadsheet: %w", err)
	}
	var sheetID int64 = -1
	for _, sh := range ss.Sheets {
		if sh.Properties.Title == eventsSheet {
			sheetID = sh.Properties.SheetId
			break
		}
	}
	if sheetID < 0 {
		return fmt.Errorf("events sheet not found")
	}

	resp, err := s.svc.Spreadsheets.Values.Get(s.spreadsheetID, eventsSheet+"!A:A").Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("get ids: %w", err)
	}
	rowIdx := -1
	for i, row := range resp.Values {
		if i == 0 {
			continue
		}
		if len(row) > 0 && fmt.Sprintf("%v", row[0]) == id {
			rowIdx = i
			break
		}
	}
	if rowIdx < 0 {
		return fmt.Errorf("event %q not found", id)
	}

	req := &googlesheets.BatchUpdateSpreadsheetRequest{
		Requests: []*googlesheets.Request{{
			DeleteDimension: &googlesheets.DeleteDimensionRequest{
				Range: &googlesheets.DimensionRange{
					SheetId:    sheetID,
					Dimension:  "ROWS",
					StartIndex: int64(rowIdx),
					EndIndex:   int64(rowIdx + 1),
				},
			},
		}},
	}
	_, err = s.svc.Spreadsheets.BatchUpdate(s.spreadsheetID, req).Context(ctx).Do()
	return err
}

// FindExistingSpreadsheet searches the user's Drive for a previously-created
// "Food Tracker — {email}" spreadsheet. Returns ("", nil) if none found.
// Uses drive.file scope so only finds files created by this app.
func FindExistingSpreadsheet(ctx context.Context, ts oauth2.TokenSource, userEmail string) (string, error) {
	driveSvc, err := drive.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return "", fmt.Errorf("drive client: %w", err)
	}
	// Escape single quotes in email (rare but possible)
	escapedEmail := strings.ReplaceAll(userEmail, "'", "\\'")
	title := fmt.Sprintf("Food Tracker \u2014 %s", escapedEmail) // \u2014 = em dash —
	q := fmt.Sprintf("name='%s' and mimeType='application/vnd.google-apps.spreadsheet' and trashed=false", title)
	list, err := driveSvc.Files.List().Q(q).Fields("files(id)").Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("drive list: %w", err)
	}
	if len(list.Files) == 0 {
		return "", nil
	}
	return list.Files[0].Id, nil
}

// AppendFood appends a food entry row.
func (s *Service) AppendFood(ctx context.Context, entry FoodEntry) error {
	vr := &googlesheets.ValueRange{Values: [][]interface{}{entry.ToRow()}}
	_, err := s.svc.Spreadsheets.Values.Append(
		s.spreadsheetID, foodSheet+"!A:J", vr,
	).ValueInputOption("RAW").Context(ctx).Do()
	return err
}

// GetFoodByDate returns all entries for a given date (YYYY-MM-DD).
func (s *Service) GetFoodByDate(ctx context.Context, date string) ([]FoodEntry, error) {
	return s.getFoodFiltered(ctx, func(d string) bool { return d == date })
}

// GetFoodByDateRange returns entries where start <= date <= end.
func (s *Service) GetFoodByDateRange(ctx context.Context, start, end string) ([]FoodEntry, error) {
	return s.getFoodFiltered(ctx, func(d string) bool { return d >= start && d <= end })
}

func (s *Service) getFoodFiltered(ctx context.Context, keep func(string) bool) ([]FoodEntry, error) {
	resp, err := s.svc.Spreadsheets.Values.Get(s.spreadsheetID, foodSheet+"!A:J").Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	var out []FoodEntry
	for i, row := range resp.Values {
		if i == 0 || len(row) < 9 {
			continue
		}
		if !keep(fmt.Sprintf("%v", row[1])) {
			continue
		}
		e, err := FoodEntryFromRow(row)
		if err != nil {
			continue
		}
		out = append(out, *e)
	}
	return out, nil
}

// UpdateFood replaces the row with the given ID.
func (s *Service) UpdateFood(ctx context.Context, id string, updated FoodEntry) error {
	resp, err := s.svc.Spreadsheets.Values.Get(s.spreadsheetID, foodSheet+"!A:A").Context(ctx).Do()
	if err != nil {
		return err
	}
	rowNum := -1
	for i, row := range resp.Values {
		if i == 0 {
			continue
		}
		if len(row) > 0 && fmt.Sprintf("%v", row[0]) == id {
			rowNum = i + 1 // 1-indexed
			break
		}
	}
	if rowNum < 0 {
		return fmt.Errorf("entry %q not found", id)
	}
	vr := &googlesheets.ValueRange{Values: [][]interface{}{updated.ToRow()}}
	_, err = s.svc.Spreadsheets.Values.Update(
		s.spreadsheetID,
		fmt.Sprintf("%s!A%d:J%d", foodSheet, rowNum, rowNum),
		vr,
	).ValueInputOption("RAW").Context(ctx).Do()
	return err
}

// DeleteFood removes the food entry row with the given ID from the Food sheet.
func (s *Service) DeleteFood(ctx context.Context, id string) error {
	// Get spreadsheet metadata to find the numeric sheetId for the Food sheet
	ss, err := s.svc.Spreadsheets.Get(s.spreadsheetID).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("get spreadsheet: %w", err)
	}
	var sheetID int64 = -1
	for _, sh := range ss.Sheets {
		if sh.Properties.Title == foodSheet {
			sheetID = sh.Properties.SheetId
			break
		}
	}
	if sheetID < 0 {
		return fmt.Errorf("food sheet not found")
	}

	// Find the row index (0-based) for the given ID
	resp, err := s.svc.Spreadsheets.Values.Get(s.spreadsheetID, foodSheet+"!A:A").Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("get ids: %w", err)
	}
	rowIdx := -1
	for i, row := range resp.Values {
		if i == 0 {
			continue // skip header
		}
		if len(row) > 0 && fmt.Sprintf("%v", row[0]) == id {
			rowIdx = i
			break
		}
	}
	if rowIdx < 0 {
		return fmt.Errorf("entry %q not found", id)
	}

	// Delete the row using batchUpdate
	req := &googlesheets.BatchUpdateSpreadsheetRequest{
		Requests: []*googlesheets.Request{{
			DeleteDimension: &googlesheets.DeleteDimensionRequest{
				Range: &googlesheets.DimensionRange{
					SheetId:    sheetID,
					Dimension:  "ROWS",
					StartIndex: int64(rowIdx),
					EndIndex:   int64(rowIdx + 1),
				},
			},
		}},
	}
	_, err = s.svc.Spreadsheets.BatchUpdate(s.spreadsheetID, req).Context(ctx).Do()
	return err
}

// GetActivity returns the DayLog for the given date, or an empty DayLog if none.
func (s *Service) GetActivity(ctx context.Context, date string) (DayLog, error) {
	resp, err := s.svc.Spreadsheets.Values.Get(s.spreadsheetID, activitySheet+"!A:G").Context(ctx).Do()
	if err != nil {
		return DayLog{}, err
	}
	for i, row := range resp.Values {
		if i == 0 || len(row) < 1 {
			continue
		}
		if fmt.Sprintf("%v", row[0]) == date {
			return DayLogFromRow(row), nil
		}
	}
	return DayLog{Date: date}, nil
}

// SetActivity upserts the DayLog for its date.
func (s *Service) SetActivity(ctx context.Context, log DayLog) error {
	resp, err := s.svc.Spreadsheets.Values.Get(s.spreadsheetID, activitySheet+"!A:A").Context(ctx).Do()
	if err != nil {
		return err
	}
	vr := &googlesheets.ValueRange{Values: [][]interface{}{log.ToRow()}}
	rowNum := -1
	for i, row := range resp.Values {
		if i == 0 {
			continue
		}
		if len(row) > 0 && fmt.Sprintf("%v", row[0]) == log.Date {
			rowNum = i + 1
			break
		}
	}
	if rowNum < 0 {
		_, err = s.svc.Spreadsheets.Values.Append(
			s.spreadsheetID, activitySheet+"!A:G", vr,
		).ValueInputOption("RAW").Context(ctx).Do()
	} else {
		_, err = s.svc.Spreadsheets.Values.Update(
			s.spreadsheetID,
			fmt.Sprintf("%s!A%d:G%d", activitySheet, rowNum, rowNum),
			vr,
		).ValueInputOption("RAW").Context(ctx).Do()
	}
	return err
}

// GetSchemaVersion reads the schema_version value from the Meta sheet.
// Returns 0 if the Meta sheet doesn't exist or has no value.
func (s *Service) GetSchemaVersion(ctx context.Context) (int, error) {
	resp, err := s.svc.Spreadsheets.Values.Get(s.spreadsheetID, metaSheet+"!A2").Context(ctx).Do()
	if err != nil {
		// 400 or 404 means sheet doesn't exist → version 0
		return 0, nil
	}
	if len(resp.Values) == 0 || len(resp.Values[0]) == 0 {
		return 0, nil
	}
	n, _ := strconv.Atoi(fmt.Sprintf("%v", resp.Values[0][0]))
	return n, nil
}

// GetActivityByDateRange returns DayLogs where start <= date <= end.
func (s *Service) GetActivityByDateRange(ctx context.Context, start, end string) ([]DayLog, error) {
	resp, err := s.svc.Spreadsheets.Values.Get(s.spreadsheetID, activitySheet+"!A:G").Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	var out []DayLog
	for i, row := range resp.Values {
		if i == 0 || len(row) < 1 {
			continue
		}
		d := fmt.Sprintf("%v", row[0])
		if d >= start && d <= end {
			out = append(out, DayLogFromRow(row))
		}
	}
	return out, nil
}
