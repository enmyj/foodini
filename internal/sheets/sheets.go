package sheets

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	googlesheets "google.golang.org/api/sheets/v4"
)

const (
	foodSheet      = "Food"
	activitySheet  = "Activity"
	metaSheet      = "Meta"
	profileSheet   = "Profile"
	insightsSheet  = "Insights"
	favoritesSheet = "Favorites"

	CurrentSchemaVersion = 8
)

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
// Stored in the Profile sheet as a single data row: gender | height | weight | notes | goals | dietary_restrictions | age
type UserProfile struct {
	Gender              string `json:"gender"`
	Height              string `json:"height"`
	Weight              string `json:"weight"`
	Notes               string `json:"notes"`
	Goals               string `json:"goals"`
	DietaryRestrictions string `json:"dietary_restrictions"`
	Age                 string `json:"age"`
}

func (p UserProfile) ToRow() []interface{} {
	return []interface{}{p.Gender, p.Height, p.Weight, p.Notes, p.Goals, p.DietaryRestrictions, p.Age}
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
		Age:                 str(6),
	}
}

// InsightRecord stores a generated AI insight in the Insights sheet.
// Schema: type | start_date | end_date | generated_at | insight
type InsightRecord struct {
	Type        string `json:"type"` // "day" or "week"
	StartDate   string `json:"start_date"`
	EndDate     string `json:"end_date"`
	GeneratedAt string `json:"generated_at"` // UTC RFC3339
	Insight     string `json:"insight"`
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
		Values: [][]interface{}{{rec.Type, rec.StartDate, rec.EndDate, rec.GeneratedAt, rec.Insight}},
	}
	_, err := s.svc.Spreadsheets.Values.Append(
		s.spreadsheetID, insightsSheet+"!A:E", vr,
	).ValueInputOption("RAW").Context(ctx).Do()
	return err
}

// GetInsight returns the most recently generated insight matching type+start+end,
// or nil if none exists. Returns nil (no error) when the Insights sheet does not exist yet.
func (s *Service) GetInsight(ctx context.Context, insightType, startDate, endDate string) (*InsightRecord, error) {
	resp, err := s.svc.Spreadsheets.Values.Get(s.spreadsheetID, insightsSheet+"!A:E").Context(ctx).Do()
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
			latest = &InsightRecord{
				Type:        str(row[0]),
				StartDate:   str(row[1]),
				EndDate:     str(row[2]),
				GeneratedAt: str(row[3]),
				Insight:     str(row[4]),
			}
		}
	}
	return latest, nil
}

// GetProfile reads the user profile from the Profile sheet.
// Returns an empty UserProfile if no data has been saved yet.
func (s *Service) GetProfile(ctx context.Context) (UserProfile, error) {
	resp, err := s.svc.Spreadsheets.Values.Get(s.spreadsheetID, profileSheet+"!A2:G2").Context(ctx).Do()
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
		s.spreadsheetID, profileSheet+"!A2:G2", vr,
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
		Values: [][]interface{}{{"gender", "height", "weight", "notes", "goals", "dietary_restrictions", "age"}},
	}
	_, err = sheetsSvc.Spreadsheets.Values.Update(
		created.SpreadsheetId, profileSheet+"!A1:G1", profHeaders,
	).ValueInputOption("RAW").Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("profile headers: %w", err)
	}

	// Insights sheet: headers row
	insightHeaders := &googlesheets.ValueRange{
		Values: [][]interface{}{{"type", "start_date", "end_date", "generated_at", "insight"}},
	}
	_, err = sheetsSvc.Spreadsheets.Values.Update(
		created.SpreadsheetId, insightsSheet+"!A1:E1", insightHeaders,
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

// MigrateSpreadsheet is an alias kept for backwards compatibility; calls MigrateV1toV2.
func MigrateSpreadsheet(ctx context.Context, ts oauth2.TokenSource, spreadsheetID string) error {
	return MigrateV1toV2(ctx, ts, spreadsheetID)
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
