# Food Tracker Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a frictionless food and activity tracker using Google OAuth2, Sheets, and Gemini — served as a single Go binary with embedded Svelte frontend.

**Architecture:** Single Go binary serves the Svelte SPA and REST API. Auth is standard Google OAuth2; Sheets and Gemini calls use the user's own OAuth token (no app-level API costs). Session is an encrypted HttpOnly cookie containing the user's refresh token and spreadsheet ID. No database.

**Tech Stack:** Go 1.22+, Svelte 5, Vite 5, `golang.org/x/oauth2`, `google.golang.org/api/sheets/v4`, `google.golang.org/api/drive/v3`, `github.com/google/generative-ai-go/genai`, `github.com/gorilla/securecookie`, `github.com/google/uuid`

---

## Directory Structure

```
foodtracker/
├── main.go
├── go.mod
├── .env.example
├── internal/
│   ├── auth/
│   │   ├── auth.go
│   │   └── auth_test.go
│   ├── sheets/
│   │   ├── sheets.go
│   │   └── sheets_test.go
│   ├── gemini/
│   │   ├── gemini.go
│   │   └── gemini_test.go
│   └── api/
│       ├── api.go
│       └── api_test.go
├── frontend/
│   ├── package.json
│   ├── vite.config.js
│   ├── src/
│   │   ├── App.svelte
│   │   ├── main.js
│   │   └── lib/
│   │       ├── api.js
│   │       ├── LogView.svelte
│   │       ├── EntryRow.svelte
│   │       ├── ChatDrawer.svelte
│   │       └── ActivityNote.svelte
│   └── dist/           ← gitignored, built output embedded by Go
└── docs/plans/
```

---

### Task 1: Project scaffold

**Files:**
- Create: `go.mod`, `main.go`, `.env.example`, `.gitignore`
- Create: `frontend/` (Svelte 5 + Vite)

**Step 1: Initialize Go module**

```bash
cd /home/imyjer/repos/foodtracker
go mod init foodtracker
```

**Step 2: Create internal package directories**

```bash
mkdir -p internal/{auth,sheets,gemini,api}
```

**Step 3: Create main.go**

```go
package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"

	"foodtracker/internal/api"
	"foodtracker/internal/auth"
)

//go:embed frontend/dist
var frontendDist embed.FS

func main() {
	cfg := auth.Config{
		ClientID:     requireEnv("GOOGLE_CLIENT_ID"),
		ClientSecret: requireEnv("GOOGLE_CLIENT_SECRET"),
		RedirectURL:  requireEnv("REDIRECT_URL"),
		CookieSecret: requireEnv("COOKIE_SECRET"),
	}

	authHandler := auth.NewHandler(cfg)
	apiHandler := api.NewHandler(authHandler)

	mux := http.NewServeMux()

	// Auth routes
	mux.HandleFunc("GET /auth/login", authHandler.Login)
	mux.HandleFunc("GET /auth/callback", authHandler.Callback)
	mux.HandleFunc("GET /auth/logout", authHandler.Logout)

	// API routes (all require auth)
	mux.HandleFunc("GET /api/log", apiHandler.Authenticated(apiHandler.GetLog))
	mux.HandleFunc("GET /api/activity", apiHandler.Authenticated(apiHandler.GetActivity))
	mux.HandleFunc("PUT /api/activity", apiHandler.Authenticated(apiHandler.PutActivity))
	mux.HandleFunc("POST /api/chat", apiHandler.Authenticated(apiHandler.Chat))
	mux.HandleFunc("PATCH /api/entries/{id}", apiHandler.Authenticated(apiHandler.PatchEntry))

	// Serve Svelte SPA — all unmatched routes serve index.html for client-side routing
	distFS, err := fs.Sub(frontendDist, "frontend/dist")
	if err != nil {
		log.Fatal(err)
	}
	mux.Handle("/", http.FileServer(http.FS(distFS)))

	port := getEnv("PORT", "8080")
	log.Printf("Listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

func requireEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("required env var %s not set", key)
	}
	return v
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
```

**Step 4: Create .env.example**

```
GOOGLE_CLIENT_ID=your-client-id.apps.googleusercontent.com
GOOGLE_CLIENT_SECRET=your-client-secret
REDIRECT_URL=http://localhost:8080/auth/callback
COOKIE_SECRET=generate-with-openssl-rand-hex-32
PORT=8080
```

**Step 5: Create .gitignore**

```
frontend/dist
frontend/node_modules
.env
foodtracker
```

**Step 6: Scaffold Svelte 5 frontend**

```bash
npm create vite@latest frontend -- --template svelte
cd frontend && npm install
# Upgrade to Svelte 5
npm install svelte@^5 @sveltejs/vite-plugin-svelte@^4
```

**Step 7: Update frontend/vite.config.js**

```js
import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'

export default defineConfig({
  plugins: [svelte()],
  server: {
    proxy: {
      '/api': 'http://localhost:8080',
      '/auth': 'http://localhost:8080',
    }
  },
  build: {
    outDir: 'dist',
  }
})
```

**Step 8: Install Go dependencies**

```bash
cd /home/imyjer/repos/foodtracker
go get golang.org/x/oauth2
go get golang.org/x/oauth2/google
go get google.golang.org/api/sheets/v4
go get google.golang.org/api/drive/v3
go get google.golang.org/api/option
go get github.com/google/generative-ai-go/genai
go get github.com/gorilla/securecookie
go get github.com/google/uuid
```

**Step 9: Commit**

```bash
git add -A
git commit -m "feat: project scaffold — Go module, Svelte 5 frontend, dependencies"
```

---

### Task 2: Auth module — Google OAuth2 + session cookie

**Files:**
- Create: `internal/auth/auth_test.go`
- Create: `internal/auth/auth.go`

**Step 1: Write failing test for session cookie round-trip**

`internal/auth/auth_test.go`:

```go
package auth_test

import (
	"net/http/httptest"
	"testing"

	"foodtracker/internal/auth"
)

func TestSessionRoundTrip(t *testing.T) {
	cfg := auth.Config{
		ClientID:     "test-id",
		ClientSecret: "test-secret",
		RedirectURL:  "http://localhost/callback",
		CookieSecret: "01234567890123456789012345678901",
	}
	h := auth.NewHandler(cfg)

	session := &auth.Session{
		UserEmail:     "test@example.com",
		RefreshToken:  "refresh-tok",
		SpreadsheetID: "sheet-123",
	}

	w := httptest.NewRecorder()
	if err := h.SetSession(w, session); err != nil {
		t.Fatalf("SetSession: %v", err)
	}

	req := httptest.NewRequest("GET", "/", nil)
	for _, c := range w.Result().Cookies() {
		req.AddCookie(c)
	}

	got, err := h.GetSession(req)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.UserEmail != session.UserEmail {
		t.Errorf("UserEmail: got %q, want %q", got.UserEmail, session.UserEmail)
	}
	if got.RefreshToken != session.RefreshToken {
		t.Errorf("RefreshToken: got %q, want %q", got.RefreshToken, session.RefreshToken)
	}
	if got.SpreadsheetID != session.SpreadsheetID {
		t.Errorf("SpreadsheetID: got %q, want %q", got.SpreadsheetID, session.SpreadsheetID)
	}
}

func TestGetSession_NoCookie(t *testing.T) {
	cfg := auth.Config{CookieSecret: "01234567890123456789012345678901"}
	h := auth.NewHandler(cfg)
	req := httptest.NewRequest("GET", "/", nil)
	_, err := h.GetSession(req)
	if err == nil {
		t.Error("expected error for missing cookie")
	}
}
```

**Step 2: Run test — expect compile failure**

```bash
go test ./internal/auth/...
```
Expected: compile error — package does not exist yet.

**Step 3: Implement internal/auth/auth.go**

```go
package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/gorilla/securecookie"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const cookieName = "ft_session"

var scopes = []string{
	"openid",
	"email",
	"profile",
	"https://www.googleapis.com/auth/spreadsheets",
	"https://www.googleapis.com/auth/drive.file",
	"https://www.googleapis.com/auth/generative-language",
}

type Config struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	CookieSecret string
}

type Session struct {
	UserEmail     string `json:"user_email"`
	RefreshToken  string `json:"refresh_token"`
	SpreadsheetID string `json:"spreadsheet_id"`
}

type Handler struct {
	oauthCfg *oauth2.Config
	sc       *securecookie.SecureCookie
}

func NewHandler(cfg Config) *Handler {
	hashKey := []byte(cfg.CookieSecret)
	encKey := []byte(cfg.CookieSecret)
	if len(encKey) > 32 {
		encKey = encKey[:32]
	}
	return &Handler{
		oauthCfg: &oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			RedirectURL:  cfg.RedirectURL,
			Scopes:       scopes,
			Endpoint:     google.Endpoint,
		},
		sc: securecookie.New(hashKey, encKey),
	}
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	url := h.oauthCfg.AuthCodeURL("state", oauth2.AccessTypeOffline, oauth2.ApprovalForce)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

func (h *Handler) Callback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	token, err := h.oauthCfg.Exchange(ctx, code)
	if err != nil {
		http.Error(w, "token exchange failed", http.StatusInternalServerError)
		return
	}

	client := h.oauthCfg.Client(ctx, token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		http.Error(w, "userinfo fetch failed", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	var info struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		http.Error(w, "userinfo decode failed", http.StatusInternalServerError)
		return
	}

	session := &Session{
		UserEmail:    info.Email,
		RefreshToken: token.RefreshToken,
	}
	if err := h.SetSession(w, session); err != nil {
		http.Error(w, "session save failed", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:    cookieName,
		Value:   "",
		Expires: time.Unix(0, 0),
		Path:    "/",
	})
	http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
}

func (h *Handler) SetSession(w http.ResponseWriter, session *Session) error {
	encoded, err := h.sc.Encode(cookieName, session)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    encoded,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   30 * 24 * 3600,
	})
	return nil
}

func (h *Handler) GetSession(r *http.Request) (*Session, error) {
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		return nil, errors.New("no session cookie")
	}
	var session Session
	if err := h.sc.Decode(cookieName, cookie.Value, &session); err != nil {
		return nil, err
	}
	return &session, nil
}

// TokenSource returns an oauth2.TokenSource for the given session's refresh token.
func (h *Handler) TokenSource(ctx context.Context, session *Session) oauth2.TokenSource {
	token := &oauth2.Token{RefreshToken: session.RefreshToken}
	return h.oauthCfg.TokenSource(ctx, token)
}

// Authenticated wraps a handler requiring a valid session.
// The session is injected into the request context.
func (h *Handler) Authenticated(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session, err := h.GetSession(r)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		r = r.WithContext(context.WithValue(r.Context(), sessionKey{}, session))
		next(w, r)
	}
}

type sessionKey struct{}

func SessionFromContext(ctx context.Context) *Session {
	s, _ := ctx.Value(sessionKey{}).(*Session)
	return s
}
```

**Step 4: Run tests**

```bash
go test ./internal/auth/...
```
Expected: PASS

**Step 5: Commit**

```bash
git add internal/auth/
git commit -m "feat: auth module — Google OAuth2, encrypted session cookie"
```

---

### Task 3: Sheets module

**Files:**
- Create: `internal/sheets/sheets_test.go`
- Create: `internal/sheets/sheets.go`

**Step 1: Write failing tests for row mapping**

`internal/sheets/sheets_test.go`:

```go
package sheets_test

import (
	"testing"
	"time"

	"foodtracker/internal/sheets"
)

func TestFoodEntryToRow(t *testing.T) {
	e := sheets.FoodEntry{
		ID:          "abc-123",
		Date:        "2026-03-05",
		Time:        "08:30",
		MealType:    "breakfast",
		Description: "oatmeal with milk",
		Calories:    300,
		Protein:     8,
		Carbs:       54,
		Fat:         6,
	}
	row := e.ToRow()
	if len(row) != 9 {
		t.Fatalf("want 9 columns, got %d", len(row))
	}
	if row[0] != "abc-123" {
		t.Errorf("col 0 (id): got %q", row[0])
	}
	if row[4] != "oatmeal with milk" {
		t.Errorf("col 4 (description): got %q", row[4])
	}
}

func TestFoodEntryFromRow(t *testing.T) {
	row := []interface{}{"abc-123", "2026-03-05", "08:30", "breakfast", "oatmeal", "300", "8", "54", "6"}
	e, err := sheets.FoodEntryFromRow(row)
	if err != nil {
		t.Fatal(err)
	}
	if e.ID != "abc-123" {
		t.Errorf("ID: got %q", e.ID)
	}
	if e.Calories != 300 {
		t.Errorf("Calories: got %d, want 300", e.Calories)
	}
	if e.MealType != "breakfast" {
		t.Errorf("MealType: got %q", e.MealType)
	}
}

func TestFoodEntryFromRow_TooShort(t *testing.T) {
	_, err := sheets.FoodEntryFromRow([]interface{}{"only", "three", "cols"})
	if err == nil {
		t.Error("expected error for short row")
	}
}

func TestDateString(t *testing.T) {
	d := sheets.DateString(time.Date(2026, 3, 5, 8, 30, 0, 0, time.UTC))
	if d != "2026-03-05" {
		t.Errorf("got %q, want 2026-03-05", d)
	}
}

func TestTimeString(t *testing.T) {
	s := sheets.TimeString(time.Date(2026, 3, 5, 8, 30, 0, 0, time.UTC))
	if s != "08:30" {
		t.Errorf("got %q, want 08:30", s)
	}
}
```

**Step 2: Run test — expect compile failure**

```bash
go test ./internal/sheets/...
```
Expected: compile error.

**Step 3: Implement internal/sheets/sheets.go**

```go
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
}

func (e FoodEntry) ToRow() []interface{} {
	return []interface{}{
		e.ID, e.Date, e.Time, e.MealType, e.Description,
		strconv.Itoa(e.Calories), strconv.Itoa(e.Protein),
		strconv.Itoa(e.Carbs), strconv.Itoa(e.Fat),
	}
}

func FoodEntryFromRow(row []interface{}) (*FoodEntry, error) {
	if len(row) < 9 {
		return nil, fmt.Errorf("row has %d columns, need 9", len(row))
	}
	str := func(v interface{}) string { return fmt.Sprintf("%v", v) }
	num := func(v interface{}) int {
		n, _ := strconv.Atoi(fmt.Sprintf("%v", v))
		return n
	}
	return &FoodEntry{
		ID: str(row[0]), Date: str(row[1]), Time: str(row[2]),
		MealType: str(row[3]), Description: str(row[4]),
		Calories: num(row[5]), Protein: num(row[6]),
		Carbs: num(row[7]), Fat: num(row[8]),
	}, nil
}

func DateString(t time.Time) string { return t.Format("2006-01-02") }
func TimeString(t time.Time) string { return t.Format("15:04") }

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
// Returns the spreadsheet ID. The Drive service is used only to verify
// the token has drive.file scope; the sheet itself is created via Sheets API.
func CreateSpreadsheet(ctx context.Context, ts oauth2.TokenSource, userEmail string) (string, error) {
	// Validate drive.file scope by listing (will fail fast if scope missing)
	driveSvc, err := drive.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return "", fmt.Errorf("drive client: %w", err)
	}
	_ = driveSvc // scope validated; actual creation via Sheets API

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
		},
	}
	created, err := sheetsSvc.Spreadsheets.Create(ss).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("create spreadsheet: %w", err)
	}

	// Write header rows
	foodHeaders := &googlesheets.ValueRange{
		Values: [][]interface{}{{"id", "date", "time", "meal_type", "description", "calories", "protein", "carbs", "fat"}},
	}
	_, err = sheetsSvc.Spreadsheets.Values.Update(
		created.SpreadsheetId, foodSheet+"!A1:I1", foodHeaders,
	).ValueInputOption("RAW").Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("food headers: %w", err)
	}

	actHeaders := &googlesheets.ValueRange{
		Values: [][]interface{}{{"date", "notes"}},
	}
	_, err = sheetsSvc.Spreadsheets.Values.Update(
		created.SpreadsheetId, activitySheet+"!A1:B1", actHeaders,
	).ValueInputOption("RAW").Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("activity headers: %w", err)
	}

	return created.SpreadsheetId, nil
}

// AppendFood appends a food entry row.
func (s *Service) AppendFood(ctx context.Context, entry FoodEntry) error {
	vr := &googlesheets.ValueRange{Values: [][]interface{}{entry.ToRow()}}
	_, err := s.svc.Spreadsheets.Values.Append(
		s.spreadsheetID, foodSheet+"!A:I", vr,
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
	resp, err := s.svc.Spreadsheets.Values.Get(s.spreadsheetID, foodSheet+"!A:I").Context(ctx).Do()
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
		fmt.Sprintf("%s!A%d:I%d", foodSheet, rowNum, rowNum),
		vr,
	).ValueInputOption("RAW").Context(ctx).Do()
	return err
}

// GetActivity returns the activity note for the given date, or "" if none.
func (s *Service) GetActivity(ctx context.Context, date string) (string, error) {
	resp, err := s.svc.Spreadsheets.Values.Get(s.spreadsheetID, activitySheet+"!A:B").Context(ctx).Do()
	if err != nil {
		return "", err
	}
	for i, row := range resp.Values {
		if i == 0 || len(row) < 2 {
			continue
		}
		if fmt.Sprintf("%v", row[0]) == date {
			return fmt.Sprintf("%v", row[1]), nil
		}
	}
	return "", nil
}

// SetActivity upserts the activity note for a given date.
func (s *Service) SetActivity(ctx context.Context, date, notes string) error {
	resp, err := s.svc.Spreadsheets.Values.Get(s.spreadsheetID, activitySheet+"!A:A").Context(ctx).Do()
	if err != nil {
		return err
	}
	vr := &googlesheets.ValueRange{Values: [][]interface{}{{date, notes}}}
	rowNum := -1
	for i, row := range resp.Values {
		if i == 0 {
			continue
		}
		if len(row) > 0 && fmt.Sprintf("%v", row[0]) == date {
			rowNum = i + 1
			break
		}
	}
	if rowNum < 0 {
		_, err = s.svc.Spreadsheets.Values.Append(
			s.spreadsheetID, activitySheet+"!A:B", vr,
		).ValueInputOption("RAW").Context(ctx).Do()
	} else {
		_, err = s.svc.Spreadsheets.Values.Update(
			s.spreadsheetID,
			fmt.Sprintf("%s!A%d:B%d", activitySheet, rowNum, rowNum),
			vr,
		).ValueInputOption("RAW").Context(ctx).Do()
	}
	return err
}
```

**Step 4: Run tests**

```bash
go test ./internal/sheets/...
```
Expected: PASS

**Step 5: Commit**

```bash
git add internal/sheets/
git commit -m "feat: sheets module — CRUD for food entries and activity notes"
```

---

### Task 4: Gemini module

**Files:**
- Create: `internal/gemini/gemini_test.go`
- Create: `internal/gemini/gemini.go`

**Step 1: Write failing tests for response parsing**

`internal/gemini/gemini_test.go`:

```go
package gemini_test

import (
	"testing"

	"foodtracker/internal/gemini"
)

func TestParseEntries_BareJSON(t *testing.T) {
	raw := `{"entries":[{"meal_type":"breakfast","description":"oatmeal","calories":300,"protein":8,"carbs":54,"fat":6}]}`
	entries, ok := gemini.ParseEntries(raw)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}
	if entries[0].MealType != "breakfast" {
		t.Errorf("MealType: got %q", entries[0].MealType)
	}
	if entries[0].Calories != 300 {
		t.Errorf("Calories: got %d", entries[0].Calories)
	}
}

func TestParseEntries_JSONInCodeFence(t *testing.T) {
	raw := "Here are your entries:\n```json\n{\"entries\":[{\"meal_type\":\"lunch\",\"description\":\"sandwich\",\"calories\":450,\"protein\":20,\"carbs\":50,\"fat\":15}]}\n```"
	entries, ok := gemini.ParseEntries(raw)
	if !ok {
		t.Fatal("expected ok=true for JSON in code fence")
	}
	if len(entries) != 1 || entries[0].MealType != "lunch" {
		t.Errorf("unexpected entries: %+v", entries)
	}
}

func TestParseEntries_Question(t *testing.T) {
	raw := `How much oatmeal did you have — about a cup?`
	_, ok := gemini.ParseEntries(raw)
	if ok {
		t.Error("expected ok=false for a plain question")
	}
}

func TestParseEntries_MultipleEntries(t *testing.T) {
	raw := `{"entries":[{"meal_type":"breakfast","description":"oatmeal","calories":300,"protein":8,"carbs":54,"fat":6},{"meal_type":"breakfast","description":"coffee","calories":5,"protein":0,"carbs":1,"fat":0}]}`
	entries, ok := gemini.ParseEntries(raw)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if len(entries) != 2 {
		t.Fatalf("want 2 entries, got %d", len(entries))
	}
}
```

**Step 2: Run test — expect compile failure**

```bash
go test ./internal/gemini/...
```
Expected: compile error.

**Step 3: Implement internal/gemini/gemini.go**

```go
package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/google/generative-ai-go/genai"
	"golang.org/x/oauth2"
	"google.golang.org/api/option"
)

const systemPrompt = `You are a food tracking assistant. The user describes what they ate.

Your job:
1. Extract food items and estimate macros (calories, protein, carbs, fat in grams).
2. If quantities are ambiguous, ask ONE short clarifying question — nothing more.
3. Once you have enough information, respond ONLY with this exact JSON format, no other text:

{"entries":[{"meal_type":"breakfast","description":"oatmeal with milk","calories":300,"protein":8,"carbs":54,"fat":6}]}

Rules:
- meal_type must be one of: breakfast, snack, lunch, dinner
- All numeric values are integers (round estimates are fine)
- Multiple foods in one meal → multiple entries, same meal_type
- Do NOT include any text outside the JSON when logging entries
- Use reasonable common serving sizes for estimates`

// Entry is a structured food log entry from Gemini.
type Entry struct {
	MealType    string `json:"meal_type"`
	Description string `json:"description"`
	Calories    int    `json:"calories"`
	Protein     int    `json:"protein"`
	Carbs       int    `json:"carbs"`
	Fat         int    `json:"fat"`
}

// ParseEntries attempts to extract a []Entry from a Gemini response.
// Returns (entries, true) if the response contains JSON entries.
// Returns (nil, false) if the response is a question or clarification.
func ParseEntries(raw string) ([]Entry, bool) {
	// Find the JSON object — handle both bare and code-fenced responses
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

// Service manages per-user Gemini conversation history.
type Service struct {
	mu    sync.Mutex
	convs map[string][]*genai.Content // keyed by userEmail
}

func NewService() *Service {
	return &Service{convs: make(map[string][]*genai.Content)}
}

// Chat sends a message and returns (responseText, entries, error).
// If Gemini returns entries, history is cleared and entries are non-nil.
// If Gemini asks a question, history is preserved and entries is nil.
func (s *Service) Chat(ctx context.Context, ts oauth2.TokenSource, userEmail, message string) (string, []Entry, error) {
	client, err := genai.NewClient(ctx, option.WithTokenSource(ts))
	if err != nil {
		return "", nil, fmt.Errorf("gemini client: %w", err)
	}
	defer client.Close()

	model := client.GenerativeModel("gemini-2.0-flash")
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
	if ok {
		// Entry confirmed — clear conversation for next log
		s.mu.Lock()
		delete(s.convs, userEmail)
		s.mu.Unlock()
		return responseText, entries, nil
	}

	// Clarifying question — persist history
	s.mu.Lock()
	s.convs[userEmail] = chatSession.History
	s.mu.Unlock()
	return responseText, nil, nil
}

// ClearConversation discards in-progress conversation for a user.
func (s *Service) ClearConversation(userEmail string) {
	s.mu.Lock()
	delete(s.convs, userEmail)
	s.mu.Unlock()
}
```

**Step 4: Run tests**

```bash
go test ./internal/gemini/...
```
Expected: PASS

**Step 5: Commit**

```bash
git add internal/gemini/
git commit -m "feat: gemini module — multi-turn conversation + structured entry extraction"
```

---

### Task 5: API handlers

**Files:**
- Create: `internal/api/api_test.go`
- Create: `internal/api/api.go`

**Step 1: Write failing test for WriteJSON helper**

`internal/api/api_test.go`:

```go
package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"foodtracker/internal/api"
)

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	api.WriteJSON(w, http.StatusOK, map[string]string{"hello": "world"})
	if w.Code != 200 {
		t.Errorf("status: got %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type: got %q", ct)
	}
	var got map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["hello"] != "world" {
		t.Errorf("body: %v", got)
	}
}

func TestWriteJSON_Status(t *testing.T) {
	w := httptest.NewRecorder()
	api.WriteJSON(w, http.StatusCreated, map[string]string{"ok": "yes"})
	if w.Code != 201 {
		t.Errorf("status: got %d, want 201", w.Code)
	}
}
```

**Step 2: Run test — expect compile failure**

```bash
go test ./internal/api/...
```
Expected: compile error.

**Step 3: Implement internal/api/api.go**

```go
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"foodtracker/internal/auth"
	"foodtracker/internal/gemini"
	"foodtracker/internal/sheets"

	"github.com/google/uuid"
)

// Handler holds references to auth and gemini services.
// The sheets service is created per-request using the user's token source.
type Handler struct {
	auth   *auth.Handler
	gemini *gemini.Service
}

func NewHandler(authHandler *auth.Handler) *Handler {
	return &Handler{
		auth:   authHandler,
		gemini: gemini.NewService(),
	}
}

// Authenticated delegates to the auth handler's middleware.
func (h *Handler) Authenticated(next http.HandlerFunc) http.HandlerFunc {
	return h.auth.Authenticated(next)
}

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	WriteJSON(w, status, map[string]string{"error": msg})
}

func (h *Handler) sheetsSvc(r *http.Request, session *auth.Session) (*sheets.Service, error) {
	ts := h.auth.TokenSource(r.Context(), session)
	return sheets.NewService(r.Context(), ts, session.SpreadsheetID)
}

// ensureSpreadsheet creates the user's spreadsheet on first login.
// Updates the session cookie with the new spreadsheet ID.
// Returns false and writes an error response if it fails.
func (h *Handler) ensureSpreadsheet(w http.ResponseWriter, r *http.Request, session *auth.Session) bool {
	if session.SpreadsheetID != "" {
		return true
	}
	ts := h.auth.TokenSource(r.Context(), session)
	id, err := sheets.CreateSpreadsheet(r.Context(), ts, session.UserEmail)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to create spreadsheet: "+err.Error())
		return false
	}
	session.SpreadsheetID = id
	if err := h.auth.SetSession(w, session); err != nil {
		writeErr(w, http.StatusInternalServerError, "session save failed")
		return false
	}
	return true
}

// GET /api/log?date=YYYY-MM-DD   → today's entries grouped with activity note
// GET /api/log?week=true          → last 7 days aggregated
func (h *Handler) GetLog(w http.ResponseWriter, r *http.Request) {
	session := auth.SessionFromContext(r.Context())
	if !h.ensureSpreadsheet(w, r, session) {
		return
	}
	svc, err := h.sheetsSvc(r, session)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	today := sheets.DateString(time.Now())

	if r.URL.Query().Get("week") == "true" {
		start := sheets.DateString(time.Now().AddDate(0, 0, -6))
		entries, err := svc.GetFoodByDateRange(r.Context(), start, today)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{
			"entries": entries,
			"start":   start,
			"end":     today,
		})
		return
	}

	date := r.URL.Query().Get("date")
	if date == "" {
		date = today
	}
	entries, err := svc.GetFoodByDate(r.Context(), date)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	activity, _ := svc.GetActivity(r.Context(), date)
	WriteJSON(w, http.StatusOK, map[string]any{
		"entries":  entries,
		"activity": activity,
		"date":     date,
	})
}

// POST /api/chat — body: {"message": "..."}
// Returns {"done": false, "message": "..."} for clarifying questions.
// Returns {"done": true, "entries": [...]} when entries are logged.
func (h *Handler) Chat(w http.ResponseWriter, r *http.Request) {
	session := auth.SessionFromContext(r.Context())
	if !h.ensureSpreadsheet(w, r, session) {
		return
	}

	var req struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Message) == "" {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ts := h.auth.TokenSource(r.Context(), session)
	responseText, entries, err := h.gemini.Chat(r.Context(), ts, session.UserEmail, req.Message)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "gemini error: "+err.Error())
		return
	}

	// Clarifying question — nothing to write to Sheets yet
	if len(entries) == 0 {
		WriteJSON(w, http.StatusOK, map[string]any{"done": false, "message": responseText})
		return
	}

	svc, err := h.sheetsSvc(r, session)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	now := time.Now()
	var saved []sheets.FoodEntry
	for _, e := range entries {
		fe := sheets.FoodEntry{
			ID:          uuid.NewString(),
			Date:        sheets.DateString(now),
			Time:        sheets.TimeString(now),
			MealType:    e.MealType,
			Description: e.Description,
			Calories:    e.Calories,
			Protein:     e.Protein,
			Carbs:       e.Carbs,
			Fat:         e.Fat,
		}
		if err := svc.AppendFood(r.Context(), fe); err != nil {
			writeErr(w, http.StatusInternalServerError, fmt.Sprintf("sheet write: %v", err))
			return
		}
		saved = append(saved, fe)
	}

	WriteJSON(w, http.StatusOK, map[string]any{"done": true, "entries": saved})
}

// PATCH /api/entries/{id} — body: FoodEntry JSON (all fields)
func (h *Handler) PatchEntry(w http.ResponseWriter, r *http.Request) {
	session := auth.SessionFromContext(r.Context())
	id := r.PathValue("id")
	if id == "" {
		writeErr(w, http.StatusBadRequest, "missing id")
		return
	}
	var entry sheets.FoodEntry
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	entry.ID = id

	svc, err := h.sheetsSvc(r, session)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := svc.UpdateFood(r.Context(), id, entry); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, entry)
}

// GET /api/activity?date=YYYY-MM-DD
func (h *Handler) GetActivity(w http.ResponseWriter, r *http.Request) {
	session := auth.SessionFromContext(r.Context())
	svc, err := h.sheetsSvc(r, session)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	date := r.URL.Query().Get("date")
	if date == "" {
		date = sheets.DateString(time.Now())
	}
	notes, err := svc.GetActivity(r.Context(), date)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, map[string]string{"date": date, "notes": notes})
}

// PUT /api/activity — body: {"date": "YYYY-MM-DD", "notes": "..."}
func (h *Handler) PutActivity(w http.ResponseWriter, r *http.Request) {
	session := auth.SessionFromContext(r.Context())
	var req struct {
		Date  string `json:"date"`
		Notes string `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Date == "" {
		req.Date = sheets.DateString(time.Now())
	}
	svc, err := h.sheetsSvc(r, session)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := svc.SetActivity(r.Context(), req.Date, req.Notes); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, map[string]string{"date": req.Date, "notes": req.Notes})
}
```

**Step 4: Run tests**

```bash
go test ./internal/api/...
```
Expected: PASS

**Step 5: Verify full Go build compiles**

```bash
go build ./...
```
Expected: no errors (main.go will fail to link until frontend/dist exists — that's fine, `go vet ./...` should pass).

**Step 6: Commit**

```bash
git add internal/api/
git commit -m "feat: API handlers — log, chat, patch entry, activity CRUD"
```

---

### Task 6: Svelte frontend — scaffold + API helper

**Files:**
- Modify: `frontend/src/App.svelte`
- Create: `frontend/src/lib/api.js`

**Step 1: Replace App.svelte**

`frontend/src/App.svelte`:

```svelte
<script>
  import { onMount } from 'svelte'
  import LogView from './lib/LogView.svelte'

  let authed = $state(null) // null=loading, false=logged out, true=logged in

  onMount(async () => {
    try {
      const res = await fetch('/api/log')
      authed = res.status !== 401
    } catch {
      authed = false
    }
  })
</script>

{#if authed === null}
  <div class="center">Loading...</div>
{:else if authed === false}
  <div class="login">
    <h1>Food Tracker</h1>
    <a href="/auth/login" class="btn">Sign in with Google</a>
  </div>
{:else}
  <LogView />
{/if}

<style>
  :global(*, *::before, *::after) { box-sizing: border-box; margin: 0; padding: 0; }
  :global(body) { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif; }
  .center { display: flex; align-items: center; justify-content: center; height: 100vh; color: #888; }
  .login { display: flex; flex-direction: column; align-items: center; justify-content: center; height: 100vh; gap: 1.5rem; }
  .login h1 { font-size: 1.75rem; color: #333; }
  .btn { padding: 0.75rem 1.5rem; background: #4285f4; color: white; border-radius: 4px; text-decoration: none; font-size: 1rem; }
</style>
```

**Step 2: Create frontend/src/lib/api.js**

```js
export async function getLog({ date = null, week = false } = {}) {
  const params = week ? '?week=true' : `?date=${date ?? 'today'}`
  const res = await fetch(`/api/log${params}`)
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function chat(message) {
  const res = await fetch('/api/chat', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ message }),
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function patchEntry(id, entry) {
  const res = await fetch(`/api/entries/${id}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(entry),
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function getActivity(date) {
  const res = await fetch(`/api/activity?date=${date}`)
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function putActivity(date, notes) {
  const res = await fetch('/api/activity', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ date, notes }),
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}
```

**Step 3: Build to verify no errors**

```bash
cd frontend && npm run build
```
Expected: `dist/` created, no errors.

**Step 4: Commit**

```bash
git add frontend/src/
git commit -m "feat: svelte scaffold — auth-aware shell and API helpers"
```

---

### Task 7: LogView + EntryRow

**Files:**
- Create: `frontend/src/lib/EntryRow.svelte`
- Create: `frontend/src/lib/LogView.svelte`

**Step 1: Create EntryRow.svelte**

`frontend/src/lib/EntryRow.svelte`:

```svelte
<script>
  import { patchEntry } from './api.js'

  let { entry, onUpdate } = $props()

  let editing = $state(null)
  let editValue = $state('')
  let saving = $state(false)

  const numFields = new Set(['calories', 'protein', 'carbs', 'fat'])

  function startEdit(field) {
    editing = field
    editValue = String(entry[field])
  }

  async function commitEdit() {
    if (!editing) return
    saving = true
    try {
      const value = numFields.has(editing) ? (parseInt(editValue) || 0) : editValue
      const saved = await patchEntry(entry.id, { ...entry, [editing]: value })
      onUpdate(saved)
    } catch (e) {
      console.error('patch failed', e)
    } finally {
      editing = null
      saving = false
    }
  }

  function onKeyDown(e) {
    if (e.key === 'Enter') commitEdit()
    if (e.key === 'Escape') editing = null
  }
</script>

<div class="row">
  <div class="desc">
    {#if editing === 'description'}
      <input bind:value={editValue} onblur={commitEdit} onkeydown={onKeyDown} autofocus />
    {:else}
      <span class="editable" onclick={() => startEdit('description')}>{entry.description}</span>
    {/if}
  </div>
  <div class="macros">
    {#each ['calories', 'protein', 'carbs', 'fat'] as field}
      {#if editing === field}
        <input class="num-input" type="number" bind:value={editValue}
               onblur={commitEdit} onkeydown={onKeyDown} autofocus />
      {:else}
        <span class="editable macro" title={field} onclick={() => startEdit(field)}>
          {entry[field]}{field === 'calories' ? ' cal' : 'g'}
        </span>
      {/if}
    {/each}
  </div>
</div>

<style>
  .row { display: flex; justify-content: space-between; align-items: center; padding: 0.5rem 0; border-bottom: 1px solid #f0f0f0; gap: 1rem; }
  .desc { flex: 1; min-width: 0; }
  .editable { cursor: pointer; border-bottom: 1px dashed transparent; }
  .editable:hover { border-bottom-color: #aaa; }
  .macros { display: flex; gap: 0.6rem; font-size: 0.82rem; color: #666; flex-shrink: 0; }
  input { border: 1px solid #4285f4; border-radius: 3px; padding: 2px 4px; font-family: inherit; }
  .num-input { width: 58px; }
</style>
```

**Step 2: Create LogView.svelte**

`frontend/src/lib/LogView.svelte`:

```svelte
<script>
  import { onMount } from 'svelte'
  import { getLog } from './api.js'
  import EntryRow from './EntryRow.svelte'
  import ChatDrawer from './ChatDrawer.svelte'
  import ActivityNote from './ActivityNote.svelte'

  const MEAL_ORDER = ['breakfast', 'snack', 'lunch', 'dinner']

  let view = $state('today')
  let data = $state(null)
  let loading = $state(true)
  let drawerOpen = $state(false)

  async function load() {
    loading = true
    try {
      data = await getLog({ week: view === 'week' })
    } finally {
      loading = false
    }
  }

  onMount(load)

  function groupedByMeal(entries) {
    const g = {}
    for (const e of entries ?? []) {
      ;(g[e.meal_type] ??= []).push(e)
    }
    return g
  }

  function groupedByDate(entries) {
    const g = {}
    for (const e of entries ?? []) {
      ;(g[e.date] ??= []).push(e)
    }
    return g
  }

  function totals(entries) {
    return (entries ?? []).reduce(
      (a, e) => ({ calories: a.calories + e.calories, protein: a.protein + e.protein, carbs: a.carbs + e.carbs, fat: a.fat + e.fat }),
      { calories: 0, protein: 0, carbs: 0, fat: 0 }
    )
  }

  function handleUpdate(updated) {
    data = { ...data, entries: data.entries.map(e => e.id === updated.id ? updated : e) }
  }

  function onEntriesAdded(newEntries) {
    data = { ...data, entries: [...(data.entries ?? []), ...newEntries] }
    drawerOpen = false
  }

  $effect(() => { if (view) load() })
</script>

<div class="wrap">
  <header>
    <div class="toggle">
      <button class:active={view === 'today'} onclick={() => view = 'today'}>Today</button>
      <button class:active={view === 'week'} onclick={() => view = 'week'}>Week</button>
    </div>
    {#if data?.entries}
      {@const t = totals(data.entries)}
      <div class="totals">
        <span>{t.calories} cal</span>
        <span>{t.protein}g P</span>
        <span>{t.carbs}g C</span>
        <span>{t.fat}g F</span>
      </div>
    {/if}
  </header>

  {#if loading}
    <p class="state">Loading…</p>
  {:else if view === 'today'}
    {#each MEAL_ORDER as meal}
      {@const group = (groupedByMeal(data?.entries)[meal] ?? [])}
      <section>
        <h3>{meal}</h3>
        {#each group as entry}
          <EntryRow {entry} onUpdate={handleUpdate} />
        {:else}
          <p class="empty">Nothing logged</p>
        {/each}
      </section>
    {/each}
    <ActivityNote date={data?.date} />
  {:else}
    {#each Object.entries(groupedByDate(data?.entries ?? [])).sort() as [date, entries]}
      {@const t = totals(entries)}
      <div class="week-row">
        <span class="date">{date}</span>
        <span>{t.calories} cal</span>
        <span>{t.protein}g P</span>
        <span>{t.carbs}g C</span>
        <span>{t.fat}g F</span>
      </div>
    {/each}
  {/if}
</div>

<button class="fab" onclick={() => drawerOpen = true} aria-label="Add food">+</button>
<ChatDrawer open={drawerOpen} onClose={() => drawerOpen = false} {onEntriesAdded} />

<style>
  .wrap { max-width: 640px; margin: 0 auto; padding: 1rem; padding-bottom: 6rem; }
  header { position: sticky; top: 0; background: white; padding: 0.75rem 0; border-bottom: 2px solid #eee; display: flex; justify-content: space-between; align-items: center; margin-bottom: 0.5rem; }
  .toggle button { padding: 0.3rem 0.85rem; border: 1px solid #ddd; background: none; cursor: pointer; font-size: 0.9rem; }
  .toggle button:first-child { border-radius: 4px 0 0 4px; }
  .toggle button:last-child { border-radius: 0 4px 4px 0; }
  .toggle button.active { background: #4285f4; color: white; border-color: #4285f4; }
  .totals { display: flex; gap: 0.75rem; font-size: 0.82rem; font-weight: 500; color: #333; }
  section { margin: 1.25rem 0; }
  h3 { text-transform: capitalize; font-size: 0.75rem; color: #999; letter-spacing: 0.06em; margin-bottom: 0.4rem; }
  .empty { color: #ccc; font-size: 0.85rem; font-style: italic; padding: 0.25rem 0; }
  .state { color: #aaa; text-align: center; margin-top: 3rem; }
  .week-row { display: flex; justify-content: space-between; padding: 0.6rem 0; border-bottom: 1px solid #f0f0f0; font-size: 0.9rem; }
  .date { font-weight: 500; }
  .fab { position: fixed; bottom: 2rem; right: 2rem; width: 3.5rem; height: 3.5rem; border-radius: 50%; background: #4285f4; color: white; font-size: 2rem; border: none; cursor: pointer; box-shadow: 0 4px 12px rgba(0,0,0,0.2); display: flex; align-items: center; justify-content: center; line-height: 1; }
</style>
```

**Step 3: Build and verify**

```bash
cd frontend && npm run build
```
Expected: no errors.

**Step 4: Commit**

```bash
git add frontend/src/lib/LogView.svelte frontend/src/lib/EntryRow.svelte
git commit -m "feat: LogView + EntryRow — today/week log with inline editing"
```

---

### Task 8: ChatDrawer

**Files:**
- Create: `frontend/src/lib/ChatDrawer.svelte`

**Step 1: Create ChatDrawer.svelte**

`frontend/src/lib/ChatDrawer.svelte`:

```svelte
<script>
  import { chat } from './api.js'

  let { open, onClose, onEntriesAdded } = $props()

  let messages = $state([])
  let input = $state('')
  let sending = $state(false)
  let inputEl = $state(null)

  $effect(() => {
    if (open) {
      setTimeout(() => inputEl?.focus(), 60)
    } else {
      messages = []
      input = ''
    }
  })

  async function send() {
    const text = input.trim()
    if (!text || sending) return
    messages = [...messages, { role: 'user', text }]
    input = ''
    sending = true
    try {
      const res = await chat(text)
      if (res.done) {
        onEntriesAdded(res.entries)
      } else {
        messages = [...messages, { role: 'assistant', text: res.message }]
      }
    } catch {
      messages = [...messages, { role: 'assistant', text: 'Something went wrong. Please try again.' }]
    } finally {
      sending = false
    }
  }

  function onKeyDown(e) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      send()
    }
  }
</script>

{#if open}
  <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
  <div class="overlay" onclick={onClose}></div>
  <div class="drawer" role="dialog" aria-label="Log food">
    <div class="handle"></div>
    <div class="messages">
      {#if messages.length === 0}
        <p class="hint">What did you eat?<br><small>e.g. "I had oatmeal and coffee for breakfast"</small></p>
      {/if}
      {#each messages as msg}
        <div class="msg {msg.role}">{msg.text}</div>
      {/each}
      {#if sending}
        <div class="msg assistant typing">…</div>
      {/if}
    </div>
    <div class="input-row">
      <textarea
        bind:this={inputEl}
        bind:value={input}
        onkeydown={onKeyDown}
        placeholder="What did you eat?"
        rows="2"
        disabled={sending}
      ></textarea>
      <button onclick={send} disabled={sending || !input.trim()}>Send</button>
    </div>
  </div>
{/if}

<style>
  .overlay { position: fixed; inset: 0; background: rgba(0,0,0,0.3); z-index: 10; }
  .drawer { position: fixed; bottom: 0; left: 0; right: 0; background: white; border-radius: 16px 16px 0 0; box-shadow: 0 -4px 24px rgba(0,0,0,0.12); z-index: 11; display: flex; flex-direction: column; max-height: 65vh; padding: 0.75rem 1rem 1.25rem; }
  .handle { width: 40px; height: 4px; background: #ddd; border-radius: 2px; margin: 0 auto 0.75rem; }
  .messages { flex: 1; overflow-y: auto; display: flex; flex-direction: column; gap: 0.5rem; margin-bottom: 0.75rem; padding: 0.25rem 0; }
  .hint { color: #bbb; font-size: 0.9rem; text-align: center; margin-top: 0.5rem; line-height: 1.6; }
  .msg { padding: 0.5rem 0.75rem; border-radius: 12px; max-width: 85%; font-size: 0.9rem; line-height: 1.4; }
  .msg.user { background: #4285f4; color: white; align-self: flex-end; }
  .msg.assistant { background: #f1f1f1; color: #333; align-self: flex-start; }
  .typing { color: #bbb; }
  .input-row { display: flex; gap: 0.5rem; align-items: flex-end; }
  textarea { flex: 1; border: 1px solid #ddd; border-radius: 8px; padding: 0.5rem; font-size: 0.95rem; resize: none; font-family: inherit; }
  textarea:focus { outline: none; border-color: #4285f4; }
  button { padding: 0.5rem 1rem; background: #4285f4; color: white; border: none; border-radius: 8px; cursor: pointer; font-size: 0.9rem; white-space: nowrap; }
  button:disabled { opacity: 0.45; cursor: default; }
</style>
```

**Step 2: Build and verify**

```bash
cd frontend && npm run build
```
Expected: no errors.

**Step 3: Commit**

```bash
git add frontend/src/lib/ChatDrawer.svelte
git commit -m "feat: ChatDrawer — Gemini multi-turn chat input"
```

---

### Task 9: ActivityNote

**Files:**
- Create: `frontend/src/lib/ActivityNote.svelte`

**Step 1: Create ActivityNote.svelte**

`frontend/src/lib/ActivityNote.svelte`:

```svelte
<script>
  import { onMount } from 'svelte'
  import { getActivity, putActivity } from './api.js'

  let { date } = $props()

  let notes = $state('')
  let editing = $state(false)
  let saving = $state(false)

  onMount(async () => {
    if (!date) return
    try {
      const res = await getActivity(date)
      notes = res.notes ?? ''
    } catch {}
  })

  async function save() {
    saving = true
    try {
      await putActivity(date, notes)
    } finally {
      saving = false
      editing = false
    }
  }

  function onKeyDown(e) {
    if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) save()
    if (e.key === 'Escape') editing = false
  }
</script>

<div class="activity">
  <h3>Activity / Notes</h3>
  {#if editing}
    <textarea
      bind:value={notes}
      onblur={save}
      onkeydown={onKeyDown}
      placeholder="What did you do today? (exercise, stress, unusual events…)"
      rows="3"
      autofocus
    ></textarea>
    {#if saving}<span class="hint">Saving…</span>{/if}
  {:else}
    <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
    <div class="note" onclick={() => editing = true}>
      {notes || 'Tap to add activity notes…'}
    </div>
  {/if}
</div>

<style>
  .activity { margin-top: 2rem; padding-top: 1.25rem; border-top: 1px solid #eee; }
  h3 { font-size: 0.75rem; color: #999; letter-spacing: 0.06em; margin-bottom: 0.5rem; }
  .note { color: #555; font-size: 0.9rem; cursor: pointer; line-height: 1.5; min-height: 1.5rem; padding: 0.2rem 0; }
  .note:hover { color: #888; }
  textarea { width: 100%; border: 1px solid #4285f4; border-radius: 6px; padding: 0.5rem; font-size: 0.9rem; font-family: inherit; resize: vertical; box-sizing: border-box; }
  textarea:focus { outline: none; }
  .hint { font-size: 0.78rem; color: #aaa; }
</style>
```

**Step 2: Build and verify**

```bash
cd frontend && npm run build
```
Expected: no errors.

**Step 3: Commit**

```bash
git add frontend/src/lib/ActivityNote.svelte
git commit -m "feat: ActivityNote — inline-editable daily activity note"
```

---

### Task 10: GCP setup + first run

**Step 1: Set up Google Cloud Project**

1. Go to https://console.cloud.google.com → create or select a project
2. **Enable APIs** (APIs & Services → Enable APIs):
   - Google Sheets API
   - Google Drive API
   - Generative Language API
3. **OAuth consent screen** (APIs & Services → OAuth consent screen):
   - User type: External
   - Fill in app name, support email
   - Add scopes: `.../auth/spreadsheets`, `.../auth/drive.file`, `.../auth/generative-language`
   - Add yourself as a test user
4. **Create OAuth credentials** (Credentials → Create → OAuth 2.0 Client ID):
   - Application type: Web application
   - Authorized redirect URIs: `http://localhost:8080/auth/callback`
   - Note the Client ID and Client Secret

**Step 2: Create .env**

```bash
cp .env.example .env
```

Fill in:
```
GOOGLE_CLIENT_ID=<from step 1>
GOOGLE_CLIENT_SECRET=<from step 1>
REDIRECT_URL=http://localhost:8080/auth/callback
COOKIE_SECRET=<output of: openssl rand -hex 32>
PORT=8080
```

**Step 3: Build frontend**

```bash
cd frontend && npm run build && cd ..
```

**Step 4: Run all Go tests**

```bash
go test ./...
```
Expected: all PASS

**Step 5: Run the app**

```bash
export $(grep -v '^#' .env | xargs) && go run main.go
```
Expected:
```
Listening on :8080
```

**Step 6: Smoke test**

1. Open http://localhost:8080 — should show "Sign in with Google"
2. Click sign in → Google OAuth consent → approve all scopes
3. Should land on the food log (empty, with meal sections)
4. Click **+** → type "I had a cup of oatmeal and black coffee for breakfast"
5. Gemini either confirms or asks one question about quantity
6. After confirming, entries appear in the Breakfast section with macro estimates
7. Click a calorie number → inline edit → press Enter → value updates
8. Scroll to bottom → tap Activity Notes → type something → click away → saves
9. Click **Week** → see today's totals in the week view
10. Open Google Drive — a "Food Tracker — your@email.com" spreadsheet should exist with your entries

**Step 7: Commit README**

Create `README.md` with GCP setup steps, env vars table, and dev/prod run instructions. Then:

```bash
git add README.md
git commit -m "docs: README with GCP setup and run instructions"
```

---

### Task 11: mise.toml — tool versions + task runner

**Files:**
- Create: `mise.toml`

mise (https://mise.jdx.dev) is a polyglot tool version manager and task runner. It pins Go and Node versions and provides `mise run <task>` commands so anyone can build/run the project without knowing the exact commands.

**Step 1: Create mise.toml**

```toml
[tools]
go = "1.22"
node = "20"

[tasks.install]
description = "Install frontend dependencies"
run = "cd frontend && npm install"

[tasks.build-frontend]
description = "Build the Svelte frontend into frontend/dist"
depends = ["install"]
run = "cd frontend && npm run build"

[tasks.build]
description = "Build production Go binary (embeds frontend)"
depends = ["build-frontend"]
run = "go build -o foodtracker ."

[tasks.dev-backend]
description = "Run Go backend in dev mode (reads .env)"
run = "export $(grep -v '^#' .env | xargs) && go run main.go"

[tasks.dev-frontend]
description = "Run Svelte dev server with API proxy on :5173"
run = "cd frontend && npm run dev"

[tasks.test]
description = "Run all Go tests"
run = "go test ./..."

[tasks.run]
description = "Build and run the production binary"
depends = ["build"]
run = "export $(grep -v '^#' .env | xargs) && ./foodtracker"

[tasks.docker-build]
description = "Build Docker image tagged as foodtracker:latest"
run = "docker build -t foodtracker ."

[tasks.docker-run]
description = "Run the app in Docker (requires .env file)"
run = "docker run --env-file .env -p 8080:8080 foodtracker"

[tasks.clean]
description = "Remove build artifacts"
run = "rm -f foodtracker && rm -rf frontend/dist"
```

**Step 2: Verify mise is available and tasks parse correctly**

```bash
mise tasks
```
Expected: all tasks listed with descriptions.

**Step 3: Verify tool versions are pinned**

```bash
mise ls
```
Expected: go 1.22 and node 20 listed.

**Step 4: Commit**

```bash
git add mise.toml
git commit -m "chore: mise.toml — tool version pins and task runner"
```

---

### Task 12: Dockerfile + .dockerignore

**Files:**
- Create: `Dockerfile`
- Create: `.dockerignore`

Multi-stage build: Node builds the Svelte frontend, Go compiles the binary with the embedded dist, a minimal distroless runtime image runs it. The final image has no shell, no package manager — just the binary.

**Step 1: Create .dockerignore**

```
.git
.env
frontend/node_modules
frontend/dist
foodtracker
*.md
docs/
```

**Step 2: Create Dockerfile**

```dockerfile
# ── Stage 1: Build Svelte frontend ────────────────────────────────────────────
FROM node:20-alpine AS frontend
WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# ── Stage 2: Build Go binary ──────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Bring in the built frontend so Go's embed directive finds it
COPY --from=frontend /app/frontend/dist ./frontend/dist
RUN CGO_ENABLED=0 GOOS=linux go build -o foodtracker .

# ── Stage 3: Minimal runtime ──────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12
COPY --from=builder /app/foodtracker /foodtracker
EXPOSE 8080
ENTRYPOINT ["/foodtracker"]
```

**Step 3: Verify the Docker build works**

```bash
docker build -t foodtracker .
```
Expected: all three stages complete, final image built. Check image size:
```bash
docker images foodtracker
```
Expected: image is well under 50MB (distroless + static binary).

**Step 4: Verify the container starts (will fail without env vars — that's expected)**

```bash
docker run --rm foodtracker
```
Expected: exits with "required env var GOOGLE_CLIENT_ID not set" — confirming the binary runs and env validation works.

**Step 5: Commit**

```bash
git add Dockerfile .dockerignore
git commit -m "chore: multi-stage Dockerfile + .dockerignore"
```

---

## Development workflow

**Hot-reload dev mode** (two terminals):

```bash
# Terminal 1: Svelte dev server with proxy
mise run dev-frontend      # http://localhost:5173

# Terminal 2: Go backend
mise run dev-backend
```

**Production build** (single binary with embedded frontend):

```bash
mise run build
mise run run
```

**Docker:**
```bash
mise run docker-build
mise run docker-run
```
