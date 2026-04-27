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
	// migratedIDs tracks spreadsheet IDs confirmed at CurrentSchemaVersion.
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
// user's spreadsheet before any handler runs. The result is memoized via
// migratedIDs so subsequent requests skip the Sheets API calls.
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

// ensureSpreadsheet finds or creates the user's spreadsheet.
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
		svc, err := sheets.NewService(r.Context(), ts, session.SpreadsheetID)
		if err != nil {
			return h.writeAPIErr(c, err)
		}
		version, err := svc.GetSchemaVersion(r.Context())
		if err != nil {
			return h.writeAPIErr(c, err)
		}
		if version < sheets.CurrentSchemaVersion {
			if err := h.runMigrations(c, ts, session.SpreadsheetID, version); err != nil {
				return err
			}
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

	if id != "" {
		svc, err := sheets.NewService(r.Context(), ts, id)
		if err != nil {
			return h.writeAPIErr(c, err)
		}
		version, err := svc.GetSchemaVersion(r.Context())
		if err != nil {
			return h.writeAPIErr(c, err)
		}
		if version < 1 {
			return writeErr(c, http.StatusConflict, "incompatible_spreadsheet")
		}
		if err := h.runMigrations(c, ts, id, version); err != nil {
			return err
		}
		session.SpreadsheetID = id
	} else {
		id, err = sheets.CreateSpreadsheet(r.Context(), ts, session.UserEmail)
		if err != nil {
			return h.writeAPIErr(c, err)
		}
		session.SpreadsheetID = id
	}

	if err := h.auth.SetSession(c, session); err != nil {
		return writeErr(c, http.StatusInternalServerError, "session save failed")
	}
	return nil
}

func (h *Handler) runMigrations(c *echo.Context, ts oauth2.TokenSource, spreadsheetID string, version int) error {
	ctx := c.Request().Context()
	type step struct {
		from    int
		migrate func(context.Context, oauth2.TokenSource, string) error
	}
	steps := []step{
		{1, sheets.MigrateV1toV2},
		{2, sheets.MigrateV2toV3},
		{3, sheets.MigrateV3toV4},
		{4, sheets.MigrateV4toV5},
		{5, sheets.MigrateV5toV6},
		{6, sheets.MigrateV6toV7},
		{7, sheets.MigrateV7toV8},
		{8, sheets.MigrateV8toV9},
		{9, sheets.MigrateV9toV10},
		{10, sheets.MigrateV10toV11},
		{11, sheets.MigrateV11toV12},
	}
	for _, s := range steps {
		if version == s.from {
			if err := s.migrate(ctx, ts, spreadsheetID); err != nil {
				return writeErr(c, http.StatusInternalServerError, "migration failed: "+err.Error())
			}
			version++
		}
	}
	return nil
}
