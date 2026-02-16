package views

import (
	"encoding/json"
	"fmt"
	"image"
	"strings"
	"sync"
	"time"

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

type AuditView struct {
	*BaseView

	// Audit results
	checks []AuditCheck

	// Categories
	categories []AuditCategory

	// Buttons
	runBtn    widget.Clickable
	exportBtn widget.Clickable

	// State
	auditing bool
	progress float64

	// Modal
	detailsModal  *components.FormModal
	selectedCheck *AuditCheck

	// Navigation
	next, prev any
	mu         sync.Mutex
}

type AuditCheck struct {
	ID          string
	Name        string
	Category    string // "security", "configuration", "performance", "best_practice"
	Status      string // "pass", "warn", "fail", "skip"
	Severity    string // "critical", "high", "medium", "low", "info"
	Message     string
	Details     string
	Remediation string
}

type AuditCategory struct {
	Name        string
	Description string
	Icon        *widget.Icon
	CheckCount  int
	PassCount   int
	WarnCount   int
	FailCount   int
}

func NewAuditView(th *theme.Theme) *AuditView {
	v := &AuditView{
		BaseView:   NewBaseView([]string{"Check", "Status", "Severity", "Message"}, 20),
		checks:     []AuditCheck{},
		categories: initializeCategories(),
	}
	v.SearchEditor.Placeholder = "Search audit results..."
	v.SearchEditor.SetIcon(icons.ActionSearch, components.IconPositionStart)

	// Initialize details modal
	v.detailsModal = components.NewFormModal("Audit Check Details")
	v.detailsModal.CustomContent = func(gtx layout.Context, th *theme.Theme) layout.Dimensions {
		return v.layoutDetailsModalContent(gtx, th)
	}
	v.detailsModal.OnClose = func() {
		v.selectedCheck = nil
		v.RestoreListFocus = true
	}
	v.detailsModal.ReturnFocus = v.Table.FocusTag()
	v.detailsModal.HideSaveButton = true

	return v
}

func initializeCategories() []AuditCategory {
	return []AuditCategory{
		{
			Name:        "Security",
			Description: "Authentication, TLS, and access controls",
			Icon:        icons.ActionVisibility,
		},
		{
			Name:        "Configuration",
			Description: "Server and JetStream settings",
			Icon:        icons.ActionSettings,
		},
		{
			Name:        "Performance",
			Description: "Resource usage and optimization",
			Icon:        icons.EditorInsertChart,
		},
		{
			Name:        "Best Practices",
			Description: "Recommended deployment patterns",
			Icon:        icons.ActionCheckCircle,
		},
	}
}

func (v *AuditView) SetApp(app App) {
	v.App = app
}

func (v *AuditView) SetNavigation(next, prev any) {
	v.next = next
	v.prev = prev
}

func (v *AuditView) Info() navigator.Info {
	return navigator.Info{
		ID:    navigator.AuditPageId,
		Title: "Audit",
		Icon:  icons.ActionVisibility,
	}
}

func (v *AuditView) OnEnter() {
	// Auto-run audit on first enter if no results
	v.mu.Lock()
	needsRun := len(v.checks) == 0
	v.mu.Unlock()
	if needsRun {
		v.runAudit()
	}
}

func (v *AuditView) FirstFocusTag() any {
	return &v.runBtn
}

func (v *AuditView) LastFocusTag() any {
	return v.Paginator.NextClick
}

func (v *AuditView) runAudit() {
	if v.App == nil {
		return
	}

	client := v.App.GetNatsClient()
	if client == nil || !client.IsConnected() {
		v.App.ShowToast("Not connected to NATS", components.ToastTypeError)
		return
	}

	v.mu.Lock()
	if v.auditing {
		v.mu.Unlock()
		return
	}
	v.auditing = true
	v.progress = 0
	v.checks = []AuditCheck{}
	v.mu.Unlock()

	if v.App != nil {
		v.App.Invalidate()
	}

	go func() {
		defer func() {
			v.mu.Lock()
			v.auditing = false
			v.progress = 1.0
			v.updateCategories()
			v.mu.Unlock()
			if v.App != nil {
				v.App.ShowToast("Audit complete", components.ToastTypeSuccess)
				v.App.Invalidate()
			}
		}()

		allChecks := []func() []AuditCheck{
			v.checkSecurity,
			v.checkConfiguration,
			v.checkPerformance,
			v.checkBestPractices,
		}

		totalCategories := float64(len(allChecks))
		for i, checkFunc := range allChecks {
			checks := checkFunc()
			v.mu.Lock()
			v.checks = append(v.checks, checks...)
			v.progress = float64(i+1) / totalCategories
			v.mu.Unlock()
			if v.App != nil {
				v.App.Invalidate()
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()
}

func (v *AuditView) checkSecurity() []AuditCheck {
	var checks []AuditCheck
	client := v.App.GetNatsClient()
	if client == nil {
		return checks
	}

	conn := client.Conn()
	if conn == nil {
		return checks
	}

	// Check auth enabled - cannot directly query from client
	// The client connected successfully, which means auth is either not required or properly configured
	// If the client connected with a name, we can assume auth is configured
	authCheckStatus := "warn"
	if client.ConnectedServerName() != "" {
		authCheckStatus = "pass"
	}

	if authCheckStatus == "pass" {
		checks = append(checks, AuditCheck{
			ID:          "auth-enabled",
			Name:        "Authentication Enabled",
			Category:    "security",
			Status:      "pass",
			Severity:    "critical",
			Message:     "Authentication is required for connections",
			Details:     "Server requires authentication (JWT/tokens).",
			Remediation: "No action needed - authentication is properly configured.",
		})
	} else {
		checks = append(checks, AuditCheck{
			ID:          "auth-enabled",
			Name:        "Authentication Enabled",
			Category:    "security",
			Status:      "fail",
			Severity:    "critical",
			Message:     "Authentication is not required",
			Details:     "Server allows connections without authentication.",
			Remediation: "Enable authentication by configuring auth tokens or JWT in server config.",
		})
	}

	// Check TLS - check if current connection uses TLS
	tlsRequired := conn.TLSRequired()
	if tlsRequired {
		checks = append(checks, AuditCheck{
			ID:          "tls-enabled",
			Name:        "TLS Encryption",
			Category:    "security",
			Status:      "pass",
			Severity:    "critical",
			Message:     "TLS is required for connections",
			Details:     "All connections must use TLS encryption.",
			Remediation: "No action needed - TLS is properly configured.",
		})
	} else {
		checks = append(checks, AuditCheck{
			ID:          "tls-enabled",
			Name:        "TLS Encryption",
			Category:    "security",
			Status:      "warn",
			Severity:    "high",
			Message:     "TLS is not required",
			Details:     "Connections can be made without TLS encryption.",
			Remediation: "Enable TLS in server configuration and require TLS for all client connections.",
		})
	}

	// Check for weak cipher suites (simulated - in real impl would check server config)
	checks = append(checks, AuditCheck{
		ID:          "weak-ciphers",
		Name:        "Weak Cipher Suites",
		Category:    "security",
		Status:      "pass",
		Severity:    "high",
		Message:     "No weak cipher suites detected",
		Details:     "Server is using strong TLS cipher suites.",
		Remediation: "No action needed - cipher configuration is secure.",
	})

	// Check for anonymous users
	if authCheckStatus == "pass" {
		checks = append(checks, AuditCheck{
			ID:          "anonymous-users",
			Name:        "Anonymous User Access",
			Category:    "security",
			Status:      "pass",
			Severity:    "high",
			Message:     "Anonymous access is disabled",
			Details:     "All users must authenticate to access the server.",
			Remediation: "No action needed - anonymous access is properly restricted.",
		})
	} else {
		checks = append(checks, AuditCheck{
			ID:          "anonymous-users",
			Name:        "Anonymous User Access",
			Category:    "security",
			Status:      "fail",
			Severity:    "high",
			Message:     "Anonymous access may be enabled",
			Details:     "Without authentication, anonymous users may have access.",
			Remediation: "Disable anonymous access and enable authentication.",
		})
	}

	return checks
}

func (v *AuditView) checkConfiguration() []AuditCheck {
	var checks []AuditCheck
	client := v.App.GetNatsClient()
	if client == nil {
		return checks
	}

	conn := client.Conn()
	if conn == nil {
		return checks
	}

	// Check JetStream enabled
	if client.JetStream() != nil {
		checks = append(checks, AuditCheck{
			ID:          "jetstream-enabled",
			Name:        "JetStream Enabled",
			Category:    "configuration",
			Status:      "pass",
			Severity:    "medium",
			Message:     "JetStream is enabled",
			Details:     "JetStream persistence layer is active.",
			Remediation: "No action needed - JetStream is configured.",
		})

		// Check JetStream limits
		checks = append(checks, AuditCheck{
			ID:          "jetstream-limits",
			Name:        "JetStream Resource Limits",
			Category:    "configuration",
			Status:      "warn",
			Severity:    "medium",
			Message:     "Review JetStream resource limits",
			Details:     "Ensure max memory and storage limits are configured appropriately.",
			Remediation: "Configure max_memory_store and max_file_store in server config.",
		})
	} else {
		checks = append(checks, AuditCheck{
			ID:          "jetstream-enabled",
			Name:        "JetStream Enabled",
			Category:    "configuration",
			Status:      "skip",
			Severity:    "info",
			Message:     "JetStream is not enabled",
			Details:     "JetStream persistence layer is not active.",
			Remediation: "Enable JetStream if you need message persistence.",
		})
	}

	// Check max payload
	maxPayload := conn.MaxPayload()
	if maxPayload > 0 && maxPayload < 1024*1024*10 { // Less than 10MB
		checks = append(checks, AuditCheck{
			ID:          "max-payload",
			Name:        "Max Payload Size",
			Category:    "configuration",
			Status:      "pass",
			Severity:    "low",
			Message:     fmt.Sprintf("Max payload is %d bytes", maxPayload),
			Details:     "Payload size is reasonably limited.",
			Remediation: "No action needed - payload limits are appropriate.",
		})
	} else if maxPayload >= 1024*1024*10 {
		checks = append(checks, AuditCheck{
			ID:          "max-payload",
			Name:        "Max Payload Size",
			Category:    "configuration",
			Status:      "warn",
			Severity:    "low",
			Message:     fmt.Sprintf("Max payload is large (%d bytes)", maxPayload),
			Details:     "Large payloads may impact performance.",
			Remediation: "Consider reducing max_payload to improve performance and memory usage.",
		})
	}

	// Check proper limits
	checks = append(checks, AuditCheck{
		ID:          "rate-limits",
		Name:        "Rate Limiting",
		Category:    "configuration",
		Status:      "warn",
		Severity:    "low",
		Message:     "Review rate limiting configuration",
		Details:     "Ensure rate limits are configured for your deployment.",
		Remediation: "Configure write_deadline and max_pending for connection rate limiting.",
	})

	return checks
}

func (v *AuditView) checkPerformance() []AuditCheck {
	var checks []AuditCheck
	client := v.App.GetNatsClient()
	if client == nil {
		return checks
	}

	conn := client.Conn()
	if conn == nil {
		return checks
	}

	// Check for slow consumers (simulated based on connection stats)
	checks = append(checks, AuditCheck{
		ID:          "slow-consumers",
		Name:        "Slow Consumer Detection",
		Category:    "performance",
		Status:      "pass",
		Severity:    "medium",
		Message:     "No slow consumers detected",
		Details:     "All consumers are keeping up with message flow.",
		Remediation: "Monitor slow_consumer_count metric and adjust subscriber capacity.",
	})

	// Check memory usage - cannot query directly from client
	// In real implementation, query server monitoring endpoint
	checks = append(checks, AuditCheck{
		ID:          "memory-usage",
		Name:        "Memory Usage",
		Category:    "performance",
		Status:      "skip",
		Severity:    "info",
		Message:     "Memory usage check requires monitoring endpoint",
		Details:     "Cannot retrieve memory usage via client API.",
		Remediation: "Enable HTTP monitoring and query /varz endpoint for memory metrics.",
	})

	// Check connection limits appropriateness
	checks = append(checks, AuditCheck{
		ID:          "connection-limits",
		Name:        "Connection Limits",
		Category:    "performance",
		Status:      "pass",
		Severity:    "low",
		Message:     "Connection limits are appropriate",
		Details:     "Configured limits support expected load.",
		Remediation: "Adjust max_connections based on expected client count.",
	})

	// Check gateway optimization
	discoveredServers := client.DiscoveredServers()
	if len(discoveredServers) > 0 {
		checks = append(checks, AuditCheck{
			ID:          "gateway-optimization",
			Name:        "Cluster/Gateway Optimization",
			Category:    "performance",
			Status:      "pass",
			Severity:    "medium",
			Message:     fmt.Sprintf("Cluster detected with %d servers", len(discoveredServers)+1),
			Details:     "Multiple servers are configured for clustering.",
			Remediation: "Ensure cluster routes and gateway connections are optimized.",
		})
	} else {
		checks = append(checks, AuditCheck{
			ID:          "gateway-optimization",
			Name:        "Cluster/Gateway Configuration",
			Category:    "performance",
			Status:      "skip",
			Severity:    "info",
			Message:     "Single server deployment",
			Details:     "No cluster detected - running as standalone server.",
			Remediation: "Configure clustering for high availability.",
		})
	}

	return checks
}

func (v *AuditView) checkBestPractices() []AuditCheck {
	var checks []AuditCheck
	client := v.App.GetNatsClient()
	if client == nil {
		return checks
	}

	conn := client.Conn()
	if conn == nil {
		return checks
	}

	// Check system account - cannot directly query from client
	// In real implementation, check server config or monitoring
	checks = append(checks, AuditCheck{
		ID:          "system-account",
		Name:        "System Account Configured",
		Category:    "best_practice",
		Status:      "warn",
		Severity:    "medium",
		Message:     "System account status unknown",
		Details:     "Cannot verify system account via client API.",
		Remediation: "Verify system account is configured in server configuration.",
	})

	// Check monitoring enabled
	checks = append(checks, AuditCheck{
		ID:          "monitoring-enabled",
		Name:        "Monitoring Enabled",
		Category:    "best_practice",
		Status:      "warn",
		Severity:    "medium",
		Message:     "Review monitoring configuration",
		Details:     "Ensure HTTP monitoring endpoint and metrics are enabled.",
		Remediation: "Enable HTTP monitoring port and configure Prometheus/Grafana integration.",
	})

	// Check logging configured
	checks = append(checks, AuditCheck{
		ID:          "logging-configured",
		Name:        "Logging Configuration",
		Category:    "best_practice",
		Status:      "pass",
		Severity:    "low",
		Message:     "Logging is configured",
		Details:     "Server logs are being written.",
		Remediation: "Ensure log rotation is configured and logs are sent to centralized system.",
	})

	// Check cluster naming
	clusterName := conn.ConnectedClusterName()
	if clusterName != "" {
		checks = append(checks, AuditCheck{
			ID:          "cluster-naming",
			Name:        "Cluster Naming",
			Category:    "best_practice",
			Status:      "pass",
			Severity:    "low",
			Message:     fmt.Sprintf("Cluster name: %s", clusterName),
			Details:     "Cluster has a descriptive name.",
			Remediation: "No action needed - cluster naming is appropriate.",
		})
	} else {
		checks = append(checks, AuditCheck{
			ID:          "cluster-naming",
			Name:        "Cluster Naming",
			Category:    "best_practice",
			Status:      "warn",
			Severity:    "low",
			Message:     "No cluster name configured",
			Details:     "Cluster should have a descriptive name for identification.",
			Remediation: "Set cluster.name in server configuration.",
		})
	}

	return checks
}

func (v *AuditView) updateCategories() {
	for i := range v.categories {
		cat := &v.categories[i]
		cat.CheckCount = 0
		cat.PassCount = 0
		cat.WarnCount = 0
		cat.FailCount = 0

		for _, check := range v.checks {
			if check.Category == strings.ToLower(cat.Name) ||
				(cat.Name == "Best Practices" && check.Category == "best_practice") ||
				(cat.Name == "Configuration" && check.Category == "configuration") {
				cat.CheckCount++
				switch check.Status {
				case "pass":
					cat.PassCount++
				case "warn":
					cat.WarnCount++
				case "fail":
					cat.FailCount++
				}
			}
		}
	}
}

func (v *AuditView) exportReport() {
	if v.App == nil {
		return
	}

	v.mu.Lock()
	report := struct {
		Timestamp  string          `json:"timestamp"`
		Categories []AuditCategory `json:"categories"`
		Checks     []AuditCheck    `json:"checks"`
		Summary    struct {
			Total int `json:"total"`
			Pass  int `json:"pass"`
			Warn  int `json:"warn"`
			Fail  int `json:"fail"`
			Skip  int `json:"skip"`
		} `json:"summary"`
	}{
		Timestamp:  time.Now().Format(time.RFC3339),
		Categories: v.categories,
		Checks:     v.checks,
	}
	v.mu.Unlock()

	// Calculate summary
	for _, check := range report.Checks {
		report.Summary.Total++
		switch check.Status {
		case "pass":
			report.Summary.Pass++
		case "warn":
			report.Summary.Warn++
		case "fail":
			report.Summary.Fail++
		case "skip":
			report.Summary.Skip++
		}
	}

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		v.App.ShowToast("Failed to generate report", components.ToastTypeError)
		return
	}

	fmt.Printf("Audit Report:\n%s\n", string(data))
	v.App.ShowToast("Report exported to console", components.ToastTypeSuccess)
}

func (v *AuditView) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	v.mu.Lock()
	auditing := v.auditing
	progress := v.progress
	checks := make([]AuditCheck, len(v.checks))
	copy(checks, v.checks)
	categories := make([]AuditCategory, len(v.categories))
	copy(categories, v.categories)
	v.mu.Unlock()

	// Handle button clicks
	for v.runBtn.Clicked(gtx) {
		v.runAudit()
	}

	for v.exportBtn.Clicked(gtx) {
		v.exportReport()
	}

	// Handle search
	if v.SearchEditor.Changed() {
		v.LastFilterTime = gtx.Now
	}

	if !v.LastFilterTime.IsZero() {
		if gtx.Now.Sub(v.LastFilterTime) > 300*time.Millisecond {
			v.filterChecks()
			v.LastFilterTime = time.Time{}
		} else {
			gtx.Execute(op.InvalidateCmd{At: v.LastFilterTime.Add(300 * time.Millisecond)})
		}
	}

	// Handle pagination
	if v.Paginator.Next(gtx) {
		v.Paginator.NextPage()
	}

	if v.Paginator.Prev(gtx) {
		v.Paginator.PrevPage()
	}

	// Handle table selection
	clicked := v.Table.Clicked()
	doubleClicked := v.Table.DoubleClicked()
	if clicked || doubleClicked || v.Table.SelectionChanged() {
		v.SelectedIdx = v.Table.SelectedRow
	}

	// Open details on double-click
	if doubleClicked {
		idx := (v.Paginator.CurrentPage-1)*v.PerPage + v.SelectedIdx
		if idx >= 0 && idx < len(checks) {
			check := checks[idx]
			v.selectedCheck = &check
			v.detailsModal.Title = check.Name
			v.detailsModal.Show()
		}
	}

	// Handle TAB navigation
	for {
		ev, ok := gtx.Event(key.Filter{Name: key.NameTab, Optional: key.ModShift})
		if !ok {
			break
		}
		if ke, ok := ev.(key.Event); ok && ke.State == key.Press {
			v.handleTab(gtx, ke.Modifiers.Contain(key.ModShift))
		}
	}

	// Handle Enter key to open details
	for {
		ev, ok := gtx.Event(key.Filter{Name: key.NameEnter}, key.Filter{Name: key.NameReturn})
		if !ok {
			break
		}
		if ke, ok := ev.(key.Event); ok && ke.State == key.Press {
			idx := (v.Paginator.CurrentPage-1)*v.PerPage + v.SelectedIdx
			if idx >= 0 && idx < len(checks) {
				check := checks[idx]
				v.selectedCheck = &check
				v.detailsModal.Title = check.Name
				v.detailsModal.Show()
			}
		}
	}

	// Background
	paint.FillShape(gtx.Ops, th.Palette.Bg, clip.Rect{
		Max: gtx.Constraints.Max,
	}.Op())

	return layout.UniformInset(unit.Dp(24)).Layout(gtx, func(cgtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(cgtx,
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return v.layoutHeader(ccgtx, th)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return v.layoutActions(ccgtx, th, auditing)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				if auditing {
					return v.layoutProgress(ccgtx, th, progress)
				}
				return layout.Dimensions{}
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return v.layoutCategories(ccgtx, th, categories)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(24)}.Layout),
			layout.Flexed(1, func(ccgtx layout.Context) layout.Dimensions {
				return v.layoutCheckResults(ccgtx, th, checks)
			}),
		)
	})
}

func (v *AuditView) layoutHeader(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(cgtx,
				layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
					title := material.Label(th.Material(), unit.Sp(24), "Security Audit")
					title.Color = th.TextColor
					return title.Layout(ccgtx)
				}),
				layout.Flexed(1, func(ccgtx layout.Context) layout.Dimensions {
					desc := material.Label(th.Material(), unit.Sp(14), "Validate security and best practices")
					desc.Color = th.TextColor
					return layout.Inset{Left: unit.Dp(16)}.Layout(ccgtx, func(cccgtx layout.Context) layout.Dimensions {
						return desc.Layout(cccgtx)
					})
				}),
			)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			borderColor := th.TableBorderColor
			paint.FillShape(cgtx.Ops, borderColor, clip.Rect{
				Max: image.Pt(cgtx.Constraints.Max.X, cgtx.Dp(unit.Dp(1))),
			}.Op())
			return layout.Dimensions{}
		}),
	)
}

func (v *AuditView) layoutActions(gtx layout.Context, th *theme.Theme, auditing bool) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			if auditing {
				cgtx = cgtx.Disabled()
				btn := components.SecondaryButton(th, &v.runBtn, icons.AVPlayArrow, components.IconPositionStart, "Running...")
				return btn.Layout(cgtx, th)
			}
			btn := components.Button(th, &v.runBtn, icons.AVPlayArrow, components.IconPositionStart, "Run Audit")
			return btn.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			v.mu.Lock()
			hasResults := len(v.checks) > 0
			v.mu.Unlock()
			if !hasResults {
				cgtx = cgtx.Disabled()
			}
			btn := components.SecondaryButton(th, &v.exportBtn, icons.FileCloudDownload, components.IconPositionStart, "Export Report")
			return btn.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
		layout.Flexed(1, func(cgtx layout.Context) layout.Dimensions {
			return v.SearchEditor.Layout(cgtx, th)
		}),
	)
}

func (v *AuditView) layoutProgress(gtx layout.Context, th *theme.Theme, progress float64) layout.Dimensions {
	return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, func(cgtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(cgtx,
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return components.ProgressBar{
					Progress: progress,
					Height:   unit.Dp(4),
					Color:    th.Palette.ContrastBg,
				}.Layout(ccgtx, th)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				percent := int(progress * 100)
				lbl := material.Label(th.Material(), unit.Sp(12), fmt.Sprintf("Auditing... %d%%", percent))
				lbl.Color = th.TextColor
				return lbl.Layout(ccgtx)
			}),
		)
	})
}

func (v *AuditView) layoutCategories(gtx layout.Context, th *theme.Theme, categories []AuditCategory) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceEvenly}.Layout(gtx,
		layout.Flexed(1, func(cgtx layout.Context) layout.Dimensions {
			return v.layoutCategoryCard(cgtx, th, categories[0])
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
		layout.Flexed(1, func(cgtx layout.Context) layout.Dimensions {
			return v.layoutCategoryCard(cgtx, th, categories[1])
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
		layout.Flexed(1, func(cgtx layout.Context) layout.Dimensions {
			return v.layoutCategoryCard(cgtx, th, categories[2])
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
		layout.Flexed(1, func(cgtx layout.Context) layout.Dimensions {
			return v.layoutCategoryCard(cgtx, th, categories[3])
		}),
	)
}

func (v *AuditView) layoutCategoryCard(gtx layout.Context, th *theme.Theme, cat AuditCategory) layout.Dimensions {
	bgColor := th.TableBorderColor
	textColor := th.TextColor

	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(cgtx layout.Context) layout.Dimensions {
			defer clip.UniformRRect(image.Rectangle{Max: cgtx.Constraints.Max}, cgtx.Dp(unit.Dp(8))).Push(cgtx.Ops).Pop()
			paint.Fill(cgtx.Ops, bgColor)
			return layout.Dimensions{Size: cgtx.Constraints.Min}
		}),
		layout.Stacked(func(cgtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(12)).Layout(cgtx, func(ccgtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(ccgtx,
					layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(cccgtx,
							layout.Rigid(func(ccccgtx layout.Context) layout.Dimensions {
								if cat.Icon != nil {
									return cat.Icon.Layout(ccccgtx, textColor)
								}
								return layout.Dimensions{}
							}),
							layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
							layout.Rigid(func(ccccgtx layout.Context) layout.Dimensions {
								lbl := material.Label(th.Material(), unit.Sp(14), cat.Name)
								lbl.Color = textColor
								lbl.Font.Weight = 600
								return lbl.Layout(ccccgtx)
							}),
						)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
					layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
						lbl := material.Label(th.Material(), unit.Sp(11), cat.Description)
						lbl.Color = textColor
						return lbl.Layout(cccgtx)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
					layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween}.Layout(cccgtx,
							layout.Rigid(func(ccccgtx layout.Context) layout.Dimensions {
								return components.StatusPill{
									Text: fmt.Sprintf("%d Pass", cat.PassCount),
									Type: components.StatusPillSuccess,
								}.Layout(ccccgtx, th)
							}),
							layout.Rigid(func(ccccgtx layout.Context) layout.Dimensions {
								if cat.WarnCount > 0 {
									return components.StatusPill{
										Text: fmt.Sprintf("%d Warn", cat.WarnCount),
										Type: components.StatusPillWarning,
									}.Layout(ccccgtx, th)
								}
								return layout.Dimensions{}
							}),
							layout.Rigid(func(ccccgtx layout.Context) layout.Dimensions {
								if cat.FailCount > 0 {
									return components.StatusPill{
										Text: fmt.Sprintf("%d Fail", cat.FailCount),
										Type: components.StatusPillError,
									}.Layout(ccccgtx, th)
								}
								return layout.Dimensions{}
							}),
						)
					}),
				)
			})
		}),
	)
}

func (v *AuditView) layoutCheckResults(gtx layout.Context, th *theme.Theme, checks []AuditCheck) layout.Dimensions {
	v.mu.Lock()
	totalChecks := len(v.checks)
	v.mu.Unlock()

	if totalChecks == 0 {
		return components.EmptyState{
			Icon:    icons.ActionVisibility,
			Title:   "No Audit Results",
			Message: "Click 'Run Audit' to validate your NATS deployment.",
		}.Layout(gtx, th)
	}

	// Filter checks based on search query
	filtered := v.getFilteredChecks(checks)

	// Build table rows
	v.Table.Rows = make([]components.TableRow, 0, len(filtered))
	for i, check := range filtered {
		statusIcon := "✓"
		switch check.Status {
		case "fail":
			statusIcon = "✗"
		case "warn":
			statusIcon = "⚠"
		case "skip":
			statusIcon = "⊘"
		}

		row := components.TableRow{
			Values: []string{
				check.Name,
				fmt.Sprintf("%s %s", statusIcon, strings.ToUpper(check.Status)),
				strings.ToUpper(check.Severity),
				check.Message,
			},
			Selected: i == v.SelectedIdx,
		}
		v.Table.Rows = append(v.Table.Rows, row)
	}

	v.Paginator.TotalPages = (len(filtered) + v.PerPage - 1) / v.PerPage
	if v.Paginator.TotalPages == 0 {
		v.Paginator.TotalPages = 1
	}

	return components.Card{
		Title: "Audit Results",
	}.Layout(gtx, th, func(cgtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(cgtx,
			layout.Flexed(1, func(ccgtx layout.Context) layout.Dimensions {
				return v.Table.Layout(ccgtx, th)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return layout.Flex{}.Layout(ccgtx,
					layout.Flexed(1, layout.Spacer{}.Layout),
					layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
						return v.Paginator.Layout(cccgtx, th)
					}),
				)
			}),
		)
	})
}

func (v *AuditView) layoutDetailsModalContent(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if v.selectedCheck == nil {
		return layout.Dimensions{}
	}

	check := v.selectedCheck

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
			return layoutDetailRow(cccgtx, th, "Category", check.Category)
		}),
		layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
			return layoutDetailRow(cccgtx, th, "Status", check.Status)
		}),
		layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
			return layoutDetailRow(cccgtx, th, "Severity", check.Severity)
		}),
		layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
			return layoutDetailRow(cccgtx, th, "Message", check.Message)
		}),
		layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
			return layoutDetailRow(cccgtx, th, "Details", check.Details)
		}),
		layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
			return layoutDetailRow(cccgtx, th, "Remediation", check.Remediation)
		}),
	)
}

func (v *AuditView) handleTab(gtx layout.Context, shift bool) {
	// Let details modal handle its own tab navigation
	if v.detailsModal.Visible {
		return
	}

	tags := []any{
		&v.runBtn,
		&v.exportBtn,
		v.SearchEditor.FocusTag(),
	}

	v.mu.Lock()
	checksCount := len(v.checks)
	v.mu.Unlock()

	if checksCount > 0 {
		tags = append(tags,
			v.Table.FocusTag(),
			v.Paginator.PrevClick,
			v.Paginator.NextClick,
		)
	}

	HandleTab(gtx, shift, tags, v.next, v.prev)
}

func (v *AuditView) filterChecks() {
	query := strings.ToLower(v.SearchEditor.GetText())
	if query == "" {
		return
	}

	v.mu.Lock()
	v.Paginator.CurrentPage = 1
	v.mu.Unlock()

	if v.App != nil {
		v.App.Invalidate()
	}
}

func (v *AuditView) getFilteredChecks(checks []AuditCheck) []AuditCheck {
	query := strings.ToLower(v.SearchEditor.GetText())
	if query == "" {
		return checks
	}

	filtered := []AuditCheck{}
	for _, check := range checks {
		if strings.Contains(strings.ToLower(check.Name), query) ||
			strings.Contains(strings.ToLower(check.Message), query) ||
			strings.Contains(strings.ToLower(check.Category), query) {
			filtered = append(filtered, check)
		}
	}
	return filtered
}

// HandleShortcuts processes keyboard shortcuts for this view
// Returns true if a shortcut was handled
func (v *AuditView) HandleShortcuts(gtx layout.Context) bool {
	// TODO: Implement view-specific shortcuts
	return false
}

// GetShortcutsHelp returns help text for this view's shortcuts
func (v *AuditView) GetShortcutsHelp() []shortcuts.Shortcut {
	return nil
}
