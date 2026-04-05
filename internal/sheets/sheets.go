
// MigrateV6toV7 upgrades an existing spreadsheet from schema v6 to v7.
// It adds the _config sheet for storing user settings (e.g. encrypted API keys).
func MigrateV6toV7(ctx context.Context, ts oauth2.TokenSource, spreadsheetID string) error {
	sheetsSvc, err := googlesheets.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return fmt.Errorf("sheets client: %w", err)
	}

	// Add _config sheet; ignore error if it already exists.
	_, _ = sheetsSvc.Spreadsheets.BatchUpdate(spreadsheetID, &googlesheets.BatchUpdateSpreadsheetRequest{
		Requests: []*googlesheets.Request{{
			AddSheet: &googlesheets.AddSheetRequest{
				Properties: &googlesheets.SheetProperties{Title: configSheet},
			},
		}},
	}).Context(ctx).Do()

	configHeaders := &googlesheets.ValueRange{
		Values: [][]interface{}{{"key", "value"}},
	}
	_, err = sheetsSvc.Spreadsheets.Values.Update(
		spreadsheetID, configSheet+"!A1:B1", configHeaders,
	).ValueInputOption("RAW").Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("migrate v6→v7 config header: %w", err)
	}

	metaData := &googlesheets.ValueRange{Values: [][]interface{}{{"7"}}}
	_, err = sheetsSvc.Spreadsheets.Values.Update(
		spreadsheetID, metaSheet+"!A2", metaData,
	).ValueInputOption("RAW").Context(ctx).Do()
	return err
}

// GetConfig reads a value from the _config sheet by key.
// Returns ("", nil) if the key is not found or the sheet does not exist.
func (s *Service) GetConfig(ctx context.Context, key string) (string, error) {
	resp, err := s.svc.Spreadsheets.Values.Get(s.spreadsheetID, configSheet+"!A:B").Context(ctx).Do()
	if err != nil {
		var ge *googleapi.Error
		if errors.As(err, &ge) && ge.Code == 400 {
			return "", nil // sheet not yet created
		}
		return "", fmt.Errorf("get config: %w", err)
	}
	for i, row := range resp.Values {
		if i == 0 || len(row) < 2 {
			continue
		}
		if fmt.Sprintf("%v", row[0]) == key {
			return fmt.Sprintf("%v", row[1]), nil
		}
	}
	return "", nil
}

// SetConfig upserts a key-value pair in the _config sheet.
func (s *Service) SetConfig(ctx context.Context, key, value string) error {
	resp, err := s.svc.Spreadsheets.Values.Get(s.spreadsheetID, configSheet+"!A:A").Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("read config keys: %w", err)
	}

	vr := &googlesheets.ValueRange{Values: [][]interface{}{{key, value}}}
	rowNum := -1
	for i, row := range resp.Values {
		if i == 0 {
			continue
		}
		if len(row) > 0 && fmt.Sprintf("%v", row[0]) == key {
			rowNum = i + 1 // 1-indexed
			break
		}
	}

	if rowNum < 0 {
		// Append new row
		_, err = s.svc.Spreadsheets.Values.Append(
			s.spreadsheetID, configSheet+"!A:B", vr,
		).ValueInputOption("RAW").Context(ctx).Do()
	} else {
		// Update existing row
		_, err = s.svc.Spreadsheets.Values.Update(
			s.spreadsheetID,
			fmt.Sprintf("%s!A%d:B%d", configSheet, rowNum, rowNum),
			vr,
		).ValueInputOption("RAW").Context(ctx).Do()
	}
	return err
}
