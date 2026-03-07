package sheets

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"golang.org/x/oauth2"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
	googlesheets "google.golang.org/api/sheets/v4"
)

const (
	foodSheet     = "Food"
	activitySheet = "Activity"
	metaSheet     = "Meta"
	profileSheet  = "Profile"

	CurrentSchemaVersion = 1
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
// Schema: date | activity | feeling_score | feeling_notes
// Backward compat: old 2-column rows (date | notes) map notes → activity.
type DayLog struct {
	Date         string `json:"date"`
	Activity     string `json:"activity"`
	FeelingScore int    `json:"feeling_score"` // 0 = not set, 1–10
	FeelingNotes string `json:"feeling_notes"`
}

func (d DayLog) ToRow() []interface{} {
	return []interface{}{
		d.Date, d.Activity, strconv.Itoa(d.FeelingScore), d.FeelingNotes,
	}
}

func DayLogFromRow(row []interface{}) DayLog {
	str := func(v interface{}) string { return fmt.Sprintf("%v", v) }
	num := func(v interface{}) int {
		n, _ := strconv.Atoi(fmt.Sprintf("%v", v))
		return n
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
	return d
}

// UserProfile stores user context for improving Gemini macro estimates.
// Stored in the Profile sheet as a single data row: gender | height | weight | notes
type UserProfile struct {
	Gender string `json:"gender"`
	Height string `json:"height"`
	Weight string `json:"weight"`
	Notes  string `json:"notes"`
}

func (p UserProfile) ToRow() []interface{} {
	return []interface{}{p.Gender, p.Height, p.Weight, p.Notes}
}

func UserProfileFromRow(row []interface{}) UserProfile {
	str := func(i int) string {
		if i < len(row) {
			return fmt.Sprintf("%v", row[i])
		}
		return ""
	}
	return UserProfile{Gender: str(0), Height: str(1), Weight: str(2), Notes: str(3)}
}

// GetProfile reads the user profile from the Profile sheet.
// Returns an empty UserProfile if no data has been saved yet.
func (s *Service) GetProfile(ctx context.Context) (UserProfile, error) {
	resp, err := s.svc.Spreadsheets.Values.Get(s.spreadsheetID, profileSheet+"!A2:D2").Context(ctx).Do()
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
		s.spreadsheetID, profileSheet+"!A2:D2", vr,
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
		Values: [][]interface{}{{"date", "activity", "feeling_score", "feeling_notes"}},
	}
	_, err = sheetsSvc.Spreadsheets.Values.Update(
		created.SpreadsheetId, activitySheet+"!A1:D1", actHeaders,
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
		Values: [][]interface{}{{"gender", "height", "weight", "notes"}},
	}
	_, err = sheetsSvc.Spreadsheets.Values.Update(
		created.SpreadsheetId, profileSheet+"!A1:D1", profHeaders,
	).ValueInputOption("RAW").Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("profile headers: %w", err)
	}

	return created.SpreadsheetId, nil
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

// GetActivity returns the DayLog for the given date, or an empty DayLog if none.
func (s *Service) GetActivity(ctx context.Context, date string) (DayLog, error) {
	resp, err := s.svc.Spreadsheets.Values.Get(s.spreadsheetID, activitySheet+"!A:D").Context(ctx).Do()
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
			s.spreadsheetID, activitySheet+"!A:D", vr,
		).ValueInputOption("RAW").Context(ctx).Do()
	} else {
		_, err = s.svc.Spreadsheets.Values.Update(
			s.spreadsheetID,
			fmt.Sprintf("%s!A%d:D%d", activitySheet, rowNum, rowNum),
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
	resp, err := s.svc.Spreadsheets.Values.Get(s.spreadsheetID, activitySheet+"!A:D").Context(ctx).Do()
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
