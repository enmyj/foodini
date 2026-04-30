package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v5"
	"golang.org/x/oauth2"
	"google.golang.org/api/googleapi"

	"foodtracker/internal/auth"
	"foodtracker/internal/gemini"
	"foodtracker/internal/sheets"
)

// Handler holds references to auth and gemini services.
type Handler struct {
	auth    *auth.Handler
	gemini  *gemini.Service
	cacheMu sync.RWMutex
	cache   map[string]cacheItem
	// migratedIDs tracks spreadsheet IDs confirmed at CurrentSchemaVersion
	// so we don't re-check the Meta sheet on every request.
	migratedMu  sync.RWMutex
	migratedIDs map[string]bool
}

type cacheItem struct {
	data    []byte
	expires time.Time
}

const cacheTTL = 60 * time.Second

func NewHandler(authHandler *auth.Handler, geminiAPIKey string) *Handler {
	return &Handler{
		auth:        authHandler,
		gemini:      gemini.NewService(geminiAPIKey),
		cache:       make(map[string]cacheItem),
		migratedIDs: make(map[string]bool),
	}
}

func (h *Handler) cacheGet(key string) ([]byte, bool) {
	h.cacheMu.RLock()
	item, ok := h.cache[key]
	h.cacheMu.RUnlock()
	if !ok || time.Now().After(item.expires) {
		return nil, false
	}
	return item.data, true
}

func (h *Handler) cacheSet(key string, data []byte) {
	h.cacheMu.Lock()
	h.cache[key] = cacheItem{data: data, expires: time.Now().Add(cacheTTL)}
	h.cacheMu.Unlock()
}

func (h *Handler) cacheInvalidate(spreadsheetID string) {
	prefix := spreadsheetID + "|"
	h.cacheMu.Lock()
	for k := range h.cache {
		if strings.HasPrefix(k, prefix) {
			delete(h.cache, k)
		}
	}
	h.cacheMu.Unlock()
}

func writeErr(c *echo.Context, status int, msg string) error {
	return c.JSON(status, map[string]string{"error": msg})
}

func isSessionExpiredErr(err error) bool {
	var re *oauth2.RetrieveError
	if errors.As(err, &re) {
		if re.ErrorCode == "invalid_grant" || re.ErrorCode == "invalid_token" {
			return true
		}
		if re.Response != nil && (re.Response.StatusCode == http.StatusBadRequest || re.Response.StatusCode == http.StatusUnauthorized) {
			return true
		}
	}
	var ge *googleapi.Error
	return errors.As(err, &ge) && ge.Code == http.StatusUnauthorized
}

func isInsufficientScopesErr(err error) bool {
	var ge *googleapi.Error
	if !errors.As(err, &ge) || ge.Code != http.StatusForbidden {
		return false
	}
	if hasInsufficientScopesText(ge.Message) || hasInsufficientScopesDetails(ge.Details) {
		return true
	}
	for _, item := range ge.Errors {
		if isInsufficientScopesReason(item.Reason) || hasInsufficientScopesText(item.Message) {
			return true
		}
	}
	return false
}

func isInsufficientScopesReason(reason string) bool {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "insufficientpermissions", "access_token_scope_insufficient":
		return true
	default:
		return false
	}
}

func hasInsufficientScopesText(msg string) bool {
	msg = strings.ToLower(strings.TrimSpace(msg))
	return strings.Contains(msg, "insufficient authentication scopes")
}

func hasInsufficientScopesDetails(details []any) bool {
	for _, detail := range details {
		if detailHasInsufficientScopes(detail) {
			return true
		}
	}
	return false
}

func detailHasInsufficientScopes(detail any) bool {
	switch v := detail.(type) {
	case map[string]any:
		for key, value := range v {
			if strings.EqualFold(key, "reason") {
				if reason, ok := value.(string); ok && isInsufficientScopesReason(reason) {
					return true
				}
			}
			if detailHasInsufficientScopes(value) {
				return true
			}
		}
	case []any:
		for _, item := range v {
			if detailHasInsufficientScopes(item) {
				return true
			}
		}
	case string:
		return isInsufficientScopesReason(v) || hasInsufficientScopesText(v)
	}
	return false
}

func (h *Handler) writeAPIErr(c *echo.Context, err error) error {
	if isSessionExpiredErr(err) {
		h.auth.ClearSession(c)
		return writeErr(c, http.StatusUnauthorized, "session_expired")
	}
	if isInsufficientScopesErr(err) {
		return writeErr(c, http.StatusForbidden, "insufficient_scopes")
	}
	return writeErr(c, http.StatusInternalServerError, err.Error())
}

// LocalNow returns the current time in the user's local timezone.
func LocalNow(r *http.Request) time.Time {
	tz := r.Header.Get("X-Timezone")
	if tz != "" {
		if loc, err := time.LoadLocation(tz); err == nil {
			return time.Now().In(loc)
		}
	}
	return time.Now()
}

func formatProfileContext(p sheets.UserProfile, currentYear int) string {
	var parts []string
	if p.BirthYear != "" {
		if by, err := strconv.Atoi(strings.TrimSpace(p.BirthYear)); err == nil && by > 1900 && by <= currentYear {
			parts = append(parts, fmt.Sprintf("age %d", currentYear-by))
		}
	}
	if p.Gender != "" {
		parts = append(parts, p.Gender)
	}
	if p.Height != "" {
		parts = append(parts, p.Height)
	}
	if p.Weight != "" {
		parts = append(parts, p.Weight)
	}
	if len(parts) == 0 && p.Notes == "" && p.Goals == "" && p.DietaryRestrictions == "" {
		return ""
	}
	ctx := "User profile: " + strings.Join(parts, ", ")
	if p.Notes != "" {
		ctx += ". " + p.Notes
	}
	if p.Goals != "" {
		ctx += ". Goals: " + p.Goals
	}
	if p.DietaryRestrictions != "" {
		ctx += ". Dietary restrictions: " + p.DietaryRestrictions
	}
	if p.NutritionExpertise != "" {
		ctx += ". Nutrition knowledge level: " + p.NutritionExpertise
	}
	return ctx
}

func (h *Handler) sheetsSvc(c *echo.Context, session *auth.Session) (*sheets.Service, error) {
	ctx := c.Request().Context()
	ts := h.auth.TokenSource(ctx, session)
	return sheets.NewService(ctx, ts, session.SpreadsheetID)
}

// EnsureSpreadsheetMiddleware returns middleware that finds or creates the
// user's spreadsheet before any handler runs.
func (h *Handler) EnsureSpreadsheetMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			session := auth.SessionFrom(c)
			if session == nil {
				return writeErr(c, http.StatusUnauthorized, "unauthorized")
			}
			if err := h.ensureSpreadsheet(c, session); err != nil {
				return err
			}
			return next(c)
		}
	}
}

// ensureSpreadsheet finds or creates the user's spreadsheet, then runs any
// pending schema migrations. Migration completion is memoized via migratedIDs
// so we only hit the Meta sheet once per process per spreadsheet.
func (h *Handler) ensureSpreadsheet(c *echo.Context, session *auth.Session) error {
	r := c.Request()
	if session.SpreadsheetID != "" {
		h.migratedMu.RLock()
		done := h.migratedIDs[session.SpreadsheetID]
		h.migratedMu.RUnlock()
		if done {
			return nil
		}
		ts := h.auth.TokenSource(r.Context(), session)
		if err := h.checkAndMigrate(c, ts, session.SpreadsheetID); err != nil {
			return err
		}
		h.migratedMu.Lock()
		h.migratedIDs[session.SpreadsheetID] = true
		h.migratedMu.Unlock()
		return nil
	}

	ts := h.auth.TokenSource(r.Context(), session)
	id, err := sheets.FindExistingSpreadsheet(r.Context(), ts, session.UserEmail)
	if err != nil {
		return h.writeAPIErr(c, err)
	}
	if id == "" {
		id, err = sheets.CreateSpreadsheet(r.Context(), ts, session.UserEmail)
		if err != nil {
			return h.writeAPIErr(c, err)
		}
	} else {
		if err := h.checkAndMigrate(c, ts, id); err != nil {
			return err
		}
	}
	session.SpreadsheetID = id
	h.migratedMu.Lock()
	h.migratedIDs[id] = true
	h.migratedMu.Unlock()
	if err := h.auth.SetSession(c, session); err != nil {
		return writeErr(c, http.StatusInternalServerError, "session save failed")
	}
	return nil
}

// checkAndMigrate reads the spreadsheet's schema version and runs any pending
// migrations. If the sheet is older than CurrentSchemaVersion and no migration
// path is registered to advance it, returns 409 unsupported_legacy_sheet so
// the frontend can prompt the user to start over with a fresh spreadsheet.
func (h *Handler) checkAndMigrate(c *echo.Context, ts oauth2.TokenSource, spreadsheetID string) error {
	ctx := c.Request().Context()
	svc, err := sheets.NewService(ctx, ts, spreadsheetID)
	if err != nil {
		return h.writeAPIErr(c, err)
	}
	version, err := svc.GetSchemaVersion(ctx)
	if err != nil {
		return h.writeAPIErr(c, err)
	}
	if version == sheets.CurrentSchemaVersion {
		return nil
	}
	final, err := h.runMigrations(c, ts, spreadsheetID, version)
	if err != nil {
		return err
	}
	if final != sheets.CurrentSchemaVersion {
		return writeErr(c, http.StatusConflict, "unsupported_legacy_sheet")
	}
	return nil
}

// runMigrations advances spreadsheetID from `version` toward CurrentSchemaVersion,
// running each registered step in order. Returns the version reached (which may
// be < CurrentSchemaVersion if no step is registered for `version`). Append a
// new entry to `steps` and bump CurrentSchemaVersion to introduce a schema change.
func (h *Handler) runMigrations(c *echo.Context, ts oauth2.TokenSource, spreadsheetID string, version int) (int, error) {
	ctx := c.Request().Context()
	type step struct {
		from    int
		migrate func(context.Context, oauth2.TokenSource, string) error
	}
	steps := []step{
		// Register future migrations here, e.g.:
		// {12, sheets.MigrateV12toV13},
	}
	for _, s := range steps {
		if version == s.from {
			if err := s.migrate(ctx, ts, spreadsheetID); err != nil {
				return version, writeErr(c, http.StatusInternalServerError, "migration failed: "+err.Error())
			}
			version++
		}
	}
	return version, nil
}
