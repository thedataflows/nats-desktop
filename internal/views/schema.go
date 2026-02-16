package views

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"regexp"
	"strings"
	"time"

	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/thedataflows/nats-desktop/internal/icons"
	"github.com/thedataflows/nats-desktop/internal/navigator"

	"github.com/thedataflows/nats-desktop/internal/shortcuts"
	"github.com/thedataflows/nats-desktop/internal/ui/components"
	"github.com/thedataflows/nats-desktop/internal/ui/theme"
)

type SchemaView struct {
	*BaseView

	schemas  []*SchemaInfo
	filtered []*SchemaInfo

	schemaEditor        *components.CodeEditor
	validationEditor    *components.CodeEditor
	validateBtn         widget.Clickable
	validateResultModal *components.FormModal
	validationResult    string
	validationError     bool

	next, prev any
}

type SchemaInfo struct {
	Name    string
	Type    string
	Version string
	Created string
	Data    string
}

func NewSchemaView(th *theme.Theme) *SchemaView {
	v := &SchemaView{
		BaseView: NewBaseView(
			[]string{"Name", "Type", "Version", "Created"},
			20,
		),
		schemaEditor:     components.NewCodeEditor("", components.CodeLanguageJSON, th),
		validationEditor: components.NewCodeEditor("", components.CodeLanguageJSON, th),
	}
	// Override default split ratio for Schema view
	v.Split.Resize.Ratio = 0.5
	v.SearchEditor.Placeholder = "Filter schemas (use * and ? wildcards)..."
	v.SearchEditor.SetIcon(icons.ActionSearch, components.IconPositionStart)

	// Initialize validation result modal
	v.validateResultModal = components.NewFormModal("Validation Result")
	v.validateResultModal.HideSaveButton = true
	v.validateResultModal.ReturnFocus = v.Table.FocusTag()
	v.validateResultModal.CustomContent = func(gtx layout.Context, th *theme.Theme) layout.Dimensions {
		return v.layoutValidationResultContent(gtx, th)
	}
	v.validateResultModal.CustomFocusTags = []event.Tag{
		v.validateResultModal.CancelBtn,
	}
	v.validateResultModal.OnClose = func() {
		v.RestoreListFocus = true
	}

	return v
}

func (v *SchemaView) SetApp(app App) {
	v.App = app
}

func (v *SchemaView) OnEnter() {
	v.Refresh()
}

func (v *SchemaView) FirstFocusTag() any {
	return &v.RefreshBtn
}

func (v *SchemaView) LastFocusTag() any {
	if v.SelectedIdx >= 0 && v.SelectedIdx < len(v.filtered) {
		return v.schemaEditor.FocusTag()
	}
	if !v.EmptyState && len(v.filtered) > 0 {
		return v.Table.FocusTag()
	}
	return v.SearchEditor.FocusTag()
}

func (v *SchemaView) SetNavigation(next, prev any) {
	v.next = next
	v.prev = prev
}

func (v *SchemaView) Info() navigator.Info {
	return navigator.Info{
		ID:    navigator.SchemaPageId,
		Title: "Schemas",
		Icon:  icons.ActionDescription,
	}
}

func (v *SchemaView) Refresh() {
	if v.App == nil {
		return
	}

	client := v.App.GetNatsClient()
	if client == nil || !client.IsConnected() {
		v.schemas = []*SchemaInfo{}
		v.EmptyState = true
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		js := client.GetJetStream()
		if js == nil {
			return
		}

		var schemas []*SchemaInfo
		streams := js.ListStreams(ctx)

		for streamInfo := range streams.Info() {
			stream, err := js.Stream(ctx, streamInfo.Config.Name)
			if err != nil {
				continue
			}

			consumers := stream.ListConsumers(ctx)

			for consumerInfo := range consumers.Info() {
				if consumerInfo.Config.FilterSubject != "" {
					schema := &SchemaInfo{
						Name:    consumerInfo.Config.FilterSubject,
						Type:    "JSON",
						Version: "1.0",
						Created: consumerInfo.Created.Format("2006-01-02"),
						Data:    fmt.Sprintf("{\n  \"$schema\": \"http://json-schema.org/draft-07/schema#\",\n  \"title\": \"%s\",\n  \"type\": \"object\",\n  \"properties\": {\n    \"id\": { \"type\": \"string\" },\n    \"timestamp\": { \"type\": \"string\", \"format\": \"date-time\" },\n    \"data\": { \"type\": \"object\" }\n  }\n}", consumerInfo.Config.FilterSubject),
					}
					schemas = append(schemas, schema)
				}
			}
		}

		v.schemas = schemas
		v.EmptyState = len(schemas) == 0
		v.filterSchemas()
		if v.App != nil && v.App.GetCurrentPageID() == navigator.SchemaPageId {
			v.App.Invalidate()
		}
	}()
}

func (v *SchemaView) filterSchemas() {
	query := v.SearchEditor.GetText()
	if query == "" {
		v.filtered = v.schemas
		v.Table.ResetWidths()
		return
	}

	// Check if query contains wildcards
	hasWildcards := strings.Contains(query, "*") || strings.Contains(query, "?")

	if hasWildcards {
		// Convert wildcard pattern to regex
		regexPattern := query
		regexPattern = strings.ReplaceAll(regexPattern, "*", ".*")
		regexPattern = strings.ReplaceAll(regexPattern, "?", ".")
		regexPattern = "(?i)^" + regexPattern + "$"

		if re, err := regexp.Compile(regexPattern); err == nil {
			v.filtered = FilterItems(v.schemas, query, func(s *SchemaInfo) bool {
				return re.MatchString(s.Name) || re.MatchString(s.Type)
			})
		} else {
			// Fallback to simple contains if regex is invalid
			lowerQuery := strings.ToLower(query)
			v.filtered = FilterItems(v.schemas, query, func(s *SchemaInfo) bool {
				return strings.Contains(strings.ToLower(s.Name), lowerQuery) ||
					strings.Contains(strings.ToLower(s.Type), lowerQuery)
			})
		}
	} else {
		// Simple substring search
		lowerQuery := strings.ToLower(query)
		v.filtered = FilterItems(v.schemas, query, func(s *SchemaInfo) bool {
			return strings.Contains(strings.ToLower(s.Name), lowerQuery) ||
				strings.Contains(strings.ToLower(s.Type), lowerQuery)
		})
	}
	v.Table.ResetWidths()
}

func (v *SchemaView) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	for v.RefreshBtn.Clicked(gtx) {
		v.Refresh()
	}

	for v.validateBtn.Clicked(gtx) {
		v.validateJSON()
	}

	for {
		ev, ok := gtx.Event(key.Filter{Name: key.NameTab, Optional: key.ModShift})
		if !ok {
			break
		}
		if ke, ok := ev.(key.Event); ok && ke.State == key.Press {
			v.handleTab(gtx, ke.Modifiers.Contain(key.ModShift))
		}
	}

	// Handle Enter key to validate selected schema
	for {
		ev, ok := gtx.Event(key.Filter{Name: key.NameEnter}, key.Filter{Name: key.NameReturn})
		if !ok {
			break
		}
		if ke, ok := ev.(key.Event); ok && ke.State == key.Press {
			if v.SelectedIdx >= 0 && v.SelectedIdx < len(v.filtered) {
				v.validateJSON()
			}
		}
	}

	if v.SearchEditor.Changed() {
		v.LastFilterTime = gtx.Now
	}

	if !v.LastFilterTime.IsZero() {
		if gtx.Now.Sub(v.LastFilterTime) > 300*time.Millisecond {
			v.filterSchemas()
			v.LastFilterTime = time.Time{}
		} else {
			gtx.Execute(op.InvalidateCmd{At: v.LastFilterTime.Add(300 * time.Millisecond)})
		}
	}

	clicked := v.Table.Clicked()
	doubleClicked := v.Table.DoubleClicked()
	if clicked || doubleClicked || v.Table.SelectionChanged() {
		v.SelectedIdx = v.Table.SelectedRow
		if v.SelectedIdx >= 0 && v.SelectedIdx < len(v.filtered) {
			v.schemaEditor.SetCode(v.filtered[v.SelectedIdx].Data)
			v.validationEditor.SetCode("{\n  \"id\": \"test-123\",\n  \"timestamp\": \"2024-01-15T10:30:00Z\",\n  \"data\": {}\n}")
		}
	}
	if doubleClicked && v.SelectedIdx >= 0 && v.SelectedIdx < len(v.filtered) {
		v.validateJSON()
	}

	paint.FillShape(gtx.Ops, th.Palette.Bg, clip.Rect{
		Max: gtx.Constraints.Max,
	}.Op())

	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(cgtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(32)).Layout(cgtx, func(ccgtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(ccgtx,
					layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
						return v.layoutHeader(cccgtx, th)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
					layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
						return v.layoutActions(cccgtx, th)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
					layout.Flexed(1, func(cccgtx layout.Context) layout.Dimensions {
						return v.layoutContent(cccgtx, th)
					}),
				)
			})
		}),
		layout.Stacked(func(cgtx layout.Context) layout.Dimensions {
			if v.validateResultModal.Visible {
				return v.validateResultModal.Layout(cgtx, th)
			}
			return layout.Dimensions{}
		}),
	)
}

func (v *SchemaView) layoutHeader(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			header := material.Label(th.Material(), unit.Sp(24), "NATS Schemas")
			header.Color = th.TextColor
			return header.Layout(cgtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			borderColor := th.TableBorderColor
			paint.FillShape(cgtx.Ops, borderColor, clip.Rect{
				Max: image.Pt(cgtx.Constraints.Max.X, cgtx.Dp(unit.Dp(1))),
			}.Op())
			return layout.Dimensions{}
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
	)
}

func (v *SchemaView) layoutActions(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			btn := components.SecondaryButton(th, &v.RefreshBtn, icons.NavigationRefresh, components.IconPositionStart, "Refresh")
			return btn.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			isSelected := v.SelectedIdx >= 0 && v.SelectedIdx < len(v.filtered)
			if !isSelected {
				cgtx = cgtx.Disabled()
			}
			btn := components.SecondaryButton(th, &v.validateBtn, icons.ActionCheckCircle, components.IconPositionStart, "Validate")
			return btn.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Flexed(1, func(cgtx layout.Context) layout.Dimensions {
			return v.SearchEditor.Layout(cgtx, th)
		}),
	)
}

func (v *SchemaView) layoutContent(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if v.EmptyState {
		return components.EmptyState{
			Icon:    icons.ActionDescription,
			Title:   "No Schemas Found",
			Message: "No schemas have been defined yet.",
		}.Layout(gtx, th)
	}

	return v.Split.Layout(gtx, th,
		func(cgtx layout.Context) layout.Dimensions {
			return v.layoutSchemaTable(cgtx, th)
		},
		func(cgtx layout.Context) layout.Dimensions {
			return v.layoutSchemaDetails(cgtx, th)
		},
	)
}

func (v *SchemaView) layoutSchemaTable(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	v.Table.Rows = make([]components.TableRow, len(v.filtered))
	for i, s := range v.filtered {
		v.Table.Rows[i] = components.TableRow{
			Values:   []string{s.Name, s.Type, s.Version, s.Created},
			Selected: i == v.SelectedIdx,
		}
	}
	return v.Table.Layout(gtx, th)
}

func (v *SchemaView) layoutSchemaDetails(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if v.SelectedIdx < 0 || v.SelectedIdx >= len(v.filtered) {
		return layout.Center.Layout(gtx, func(cgtx layout.Context) layout.Dimensions {
			lbl := material.Label(th.Material(), unit.Sp(14), "Select a schema to see details")
			lbl.Color = th.TextColor
			return lbl.Layout(cgtx)
		})
	}

	schema := v.filtered[v.SelectedIdx]
	return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(cgtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(cgtx,
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				header := material.Label(th.Material(), unit.Sp(18), "Schema: "+schema.Name)
				header.Color = th.TextColor
				header.Font.Weight = font.Bold
				return header.Layout(ccgtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return layoutDetailRow(ccgtx, th, "Type", schema.Type)
			}),
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return layoutDetailRow(ccgtx, th, "Version", schema.Version)
			}),
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return layoutDetailRow(ccgtx, th, "Created", schema.Created)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Flexed(1, func(ccgtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(ccgtx,
					layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
						return components.Card{
							Title:    "Schema Definition",
							Flexible: true,
						}.Layout(cccgtx, th, func(c4gtx layout.Context) layout.Dimensions {
							return v.schemaEditor.Layout(c4gtx, th)
						})
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
					layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
						return components.Card{
							Title:    "Test JSON (to validate)",
							Flexible: true,
						}.Layout(cccgtx, th, func(c4gtx layout.Context) layout.Dimensions {
							return v.validationEditor.Layout(c4gtx, th)
						})
					}),
				)
			}),
		)
	})
}

func (v *SchemaView) handleTab(gtx layout.Context, shift bool) {
	// Let validation result modal handle its own tab navigation
	if v.validateResultModal.Visible {
		return
	}

	tags := []any{
		&v.RefreshBtn,
		&v.validateBtn,
		v.SearchEditor.FocusTag(),
	}

	if !v.EmptyState && len(v.filtered) > 0 {
		tags = append(tags, v.Table.FocusTag())

		if v.SelectedIdx >= 0 && v.SelectedIdx < len(v.filtered) {
			tags = append(tags, v.schemaEditor.FocusTag())
			tags = append(tags, v.validationEditor.FocusTag())
		}
	}

	HandleTab(gtx, shift, tags, v.next, v.prev)
}

// validateJSON performs basic JSON validation against the selected schema
func (v *SchemaView) validateJSON() {
	if v.SelectedIdx < 0 || v.SelectedIdx >= len(v.filtered) {
		return
	}

	schemaData := v.schemaEditor.GetCode()
	testData := v.validationEditor.GetCode()

	// Check if test JSON is empty
	testData = strings.TrimSpace(testData)
	if testData == "" {
		v.validationResult = "Please enter JSON data to validate"
		v.validationError = true
		v.validateResultModal.Title = "Validation Failed"
		v.validateResultModal.Show()
		if v.App != nil {
			v.App.Invalidate()
		}
		return
	}

	// Parse test JSON
	var testObj interface{}
	if err := json.Unmarshal([]byte(testData), &testObj); err != nil {
		v.validationResult = "Invalid JSON syntax: " + err.Error()
		v.validationError = true
		v.validateResultModal.Title = "Validation Failed"
		v.validateResultModal.Show()
		if v.App != nil {
			v.App.Invalidate()
		}
		return
	}

	// If no schema, just validate JSON syntax
	schemaData = strings.TrimSpace(schemaData)
	if schemaData == "" {
		v.validationResult = "JSON syntax is valid"
		v.validationError = false
		v.validateResultModal.Title = "Validation Passed"
		v.validateResultModal.Show()
		if v.App != nil {
			v.App.Invalidate()
		}
		return
	}

	// Parse schema
	var schemaObj map[string]interface{}
	if err := json.Unmarshal([]byte(schemaData), &schemaObj); err != nil {
		v.validationResult = "Schema is not valid JSON: " + err.Error()
		v.validationError = true
		v.validateResultModal.Title = "Validation Failed"
		v.validateResultModal.Show()
		if v.App != nil {
			v.App.Invalidate()
		}
		return
	}

	// Check schema type
	schemaType, _ := schemaObj["type"].(string)
	if schemaType != "" && schemaType != "object" {
		// For non-object types, just check the test data type matches
		valid := false
		switch schemaType {
		case "string":
			_, valid = testObj.(string)
		case "number":
			_, isFloat := testObj.(float64)
			valid = isFloat
		case "integer":
			if f, ok := testObj.(float64); ok {
				valid = f == float64(int64(f))
			}
		case "boolean":
			_, valid = testObj.(bool)
		case "array":
			_, valid = testObj.([]interface{})
		}
		if !valid {
			v.validationResult = fmt.Sprintf("Expected type '%s'", schemaType)
			v.validationError = true
			v.validateResultModal.Title = "Validation Failed"
		} else {
			v.validationResult = "JSON is valid"
			v.validationError = false
			v.validateResultModal.Title = "Validation Passed"
		}
		v.validateResultModal.Show()
		if v.App != nil {
			v.App.Invalidate()
		}
		return
	}

	// For object type, check required fields if present
	testMap, isMap := testObj.(map[string]interface{})
	if !isMap {
		v.validationResult = "Schema expects an object, but data is not an object"
		v.validationError = true
		v.validateResultModal.Title = "Validation Failed"
		v.validateResultModal.Show()
		if v.App != nil {
			v.App.Invalidate()
		}
		return
	}

	// Check required fields
	if required, ok := schemaObj["required"].([]interface{}); ok && len(required) > 0 {
		missingFields := []string{}
		for _, r := range required {
			if field, ok := r.(string); ok {
				if _, exists := testMap[field]; !exists {
					missingFields = append(missingFields, field)
				}
			}
		}
		if len(missingFields) > 0 {
			v.validationResult = fmt.Sprintf("Missing required fields: %s", strings.Join(missingFields, ", "))
			v.validationError = true
			v.validateResultModal.Title = "Validation Failed"
			v.validateResultModal.Show()
			if v.App != nil {
				v.App.Invalidate()
			}
			return
		}
	}

	// All checks passed
	v.validationResult = "JSON is valid according to the schema"
	v.validationError = false
	v.validateResultModal.Title = "Validation Passed"
	v.validateResultModal.Show()
	if v.App != nil {
		v.App.Invalidate()
	}
}

// layoutValidationResultContent renders the validation result in the modal
func (v *SchemaView) layoutValidationResultContent(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	resultColor := th.Palette.ContrastFg
	if v.validationError {
		resultColor = th.ErrorColor
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			lbl := material.Label(th.Material(), unit.Sp(14), v.validationResult)
			lbl.Color = resultColor
			return lbl.Layout(cgtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			lbl := material.Label(th.Material(), unit.Sp(12), "Press ESC or click Close to dismiss")
			lbl.Color = th.SecondaryTextColor
			return lbl.Layout(cgtx)
		}),
	)
}

// HandleShortcuts processes keyboard shortcuts for this view
// Returns true if a shortcut was handled
func (v *SchemaView) HandleShortcuts(gtx layout.Context) bool {
	// TODO: Implement view-specific shortcuts
	return false
}

// GetShortcutsHelp returns help text for this view's shortcuts
func (v *SchemaView) GetShortcutsHelp() []shortcuts.Shortcut {
	return nil
}
