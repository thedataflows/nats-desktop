package views

import (
	"fmt"
	"image"
	"strconv"
	"time"

	"github.com/thedataflows/nats-desktop/internal/config"
	"github.com/thedataflows/nats-desktop/internal/embedded"
	"github.com/thedataflows/nats-desktop/internal/ui/components"
	"github.com/thedataflows/nats-desktop/internal/ui/theme"

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
	"gioui.org/x/component"
	log "github.com/thedataflows/go-lib-log"

	"github.com/thedataflows/nats-desktop/internal/icons"
	"github.com/thedataflows/nats-desktop/internal/nats"
	"github.com/thedataflows/nats-desktop/internal/navigator"
	"github.com/thedataflows/nats-desktop/internal/shortcuts"
)

type ConnectionsView struct {
	app               App
	addBtn            widget.Clickable
	editBtn           widget.Clickable
	removeBtn         widget.Clickable
	refreshBtn        widget.Clickable
	connectBtn        widget.Clickable
	importBtn         widget.Clickable
	exportBtn         widget.Clickable
	switchBtn         widget.Clickable
	prevCtxBtn        widget.Clickable
	embeddedServerBtn widget.Clickable

	selectedIdx int

	addModal            *components.FormModal
	removeModal         *components.Prompt
	editModal           *components.FormModal
	importModal         *components.Prompt
	exportModal         *components.Prompt
	embeddedServerModal *components.FormModal

	nameEditor        *components.InputField
	urlEditor         *components.InputField
	descriptionEditor *components.InputField
	usernameEditor    *components.InputField
	passwordEditor    *components.InputField
	tokenEditor       *components.InputField
	nkeyEditor        *components.InputField
	credsEditor       *components.InputField
	jsDirEditor       *components.InputField
	portEditor        *components.InputField

	authSection *components.ExpandableSection

	clickableList  []widget.Clickable
	listState      *widget.List
	listFocus      widget.Clickable
	loadingOverlay components.LoadingOverlay

	natsClient *nats.Client
	split      components.SplitView

	focusRequested bool

	next, prev any

	embeddedServer    *embedded.Server
	embeddedRunning   bool
	embeddedConnected bool
}

func (v *ConnectionsView) GetNatsClient() *nats.Client {
	return v.natsClient
}

func (v *ConnectionsView) Refresh() {
	if v.app == nil {
		return
	}

	cfg := v.app.GetConfig()
	var connectedCtx *config.ContextInfo
	for _, ctx := range cfg.Contexts {
		if ctx.Connected {
			connectedCtx = ctx
			break
		}
	}

	if connectedCtx == nil {
		return
	}

	v.loadingOverlay.Loading = true
	v.loadingOverlay.Message = "Refreshing connections..."
	go func() {
		defer func() {
			v.loadingOverlay.Loading = false
			if v.app != nil && v.app.GetCurrentPageID() == navigator.ConnectionsPageId {
				v.app.Invalidate()
			}
		}()

		if v.natsClient != nil {
			if !v.natsClient.IsConnected() {
				v.natsClient.Close()
				v.natsClient = nil
				connectedCtx.Connected = false
				v.app.SetStatus("Disconnected", false)
				v.app.SetContextName("")
				v.app.UpdateStatusText("Disconnected")
				v.app.ShowToast("Connection lost to "+connectedCtx.Name, components.ToastTypeError)
			}
		}

		time.Sleep(500 * time.Millisecond)
	}()
}

func NewConnectionsView(th *theme.Theme) *ConnectionsView {
	nameEditor := components.NewLabeledInputField("Name", "")
	urlEditor := components.NewLabeledInputField("URL", "")
	descriptionEditor := components.NewLabeledInputField("Description", "")
	usernameEditor := components.NewLabeledInputField("Username", "")
	passwordEditor := components.NewLabeledInputField("Password", "")
	tokenEditor := components.NewLabeledInputField("Token", "")
	nkeyEditor := components.NewLabeledInputField("NKey File", "")
	credsEditor := components.NewLabeledInputField("Credentials File", "")
	jsDirEditor := components.NewLabeledInputField("JetStream Directory", "./")
	jsDirEditor.SetLabelWidth(unit.Dp(120))
	portEditor := components.NewLabeledInputField("Port", "4222")
	portEditor.SetLabelWidth(unit.Dp(120))

	v := &ConnectionsView{
		nameEditor:        nameEditor,
		urlEditor:         urlEditor,
		descriptionEditor: descriptionEditor,
		usernameEditor:    usernameEditor,
		passwordEditor:    passwordEditor,
		tokenEditor:       tokenEditor,
		nkeyEditor:        nkeyEditor,
		credsEditor:       credsEditor,
		jsDirEditor:       jsDirEditor,
		portEditor:        portEditor,
		authSection:       components.NewExpandableSection("Authentication (Optional)"),
		removeModal: components.NewPrompt("Remove Context", "Are you sure you want to remove this context?", components.ModalTypeWarn,
			components.Option{Text: "Remove"},
			components.Option{Text: "Cancel"},
		),
		addModal:            components.NewFormModal("Add Context"),
		editModal:           components.NewFormModal("Edit Context"),
		embeddedServerModal: components.NewFormModal("Start Embedded Server"),
		importModal: components.NewPrompt("Import from natscli",
			"Import all contexts from ~/.config/nats/context/?",
			components.ModalTypeInfo,
			components.Option{Text: "Import"},
			components.Option{Text: "Cancel"},
		),
		exportModal: components.NewPrompt("Export to natscli",
			"Export all contexts to ~/.config/nats/context/?",
			components.ModalTypeInfo,
			components.Option{Text: "Export"},
			components.Option{Text: "Cancel"},
		),
		listState: &widget.List{
			List: layout.List{
				Axis: layout.Vertical,
			},
		},
		split: components.SplitView{
			Resize: component.Resize{
				Ratio: 0.5,
			},
		},
	}

	v.addModal.ReturnFocus = &v.listFocus
	v.addModal.MaxWidth = unit.Dp(350)
	v.editModal.ReturnFocus = &v.listFocus
	v.editModal.MaxWidth = unit.Dp(350)
	v.removeModal.ReturnFocus = &v.listFocus
	v.importModal.ReturnFocus = &v.listFocus
	v.exportModal.ReturnFocus = &v.listFocus

	// Set up custom content for add modal with expandable auth section
	v.addModal.CustomContent = func(gtx layout.Context, th *theme.Theme) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
				return v.nameEditor.Layout(cgtx, th)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
				return v.descriptionEditor.Layout(cgtx, th)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
				return v.urlEditor.Layout(cgtx, th)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
				return v.authSection.Layout(cgtx, th, func(ccgtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(ccgtx,
						layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
							return v.usernameEditor.Layout(cccgtx, th)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
						layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
							return v.passwordEditor.Layout(cccgtx, th)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
						layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
							return v.tokenEditor.Layout(cccgtx, th)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
						layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
							return v.nkeyEditor.Layout(cccgtx, th)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
						layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
							return v.credsEditor.Layout(cccgtx, th)
						}),
					)
				})
			}),
		)
	}
	v.addModal.CustomFocusTagsFunc = func() []event.Tag {
		tags := []event.Tag{
			v.nameEditor.FocusTag(),
			v.descriptionEditor.FocusTag(),
			v.urlEditor.FocusTag(),
			v.authSection.FocusTag(),
		}
		if v.authSection.Expanded {
			tags = append(tags,
				v.usernameEditor.FocusTag(),
				v.passwordEditor.FocusTag(),
				v.tokenEditor.FocusTag(),
				v.nkeyEditor.FocusTag(),
				v.credsEditor.FocusTag(),
			)
		}
		return tags
	}
	v.addModal.OnSave = func() bool {
		return v.saveContext()
	}
	v.addModal.OnClose = func() {
		v.focusRequested = true
	}

	// Set up custom content for edit modal with expandable auth section
	v.editModal.CustomContent = func(gtx layout.Context, th *theme.Theme) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
				return v.nameEditor.Layout(cgtx, th)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
				return v.descriptionEditor.Layout(cgtx, th)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
				return v.urlEditor.Layout(cgtx, th)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
				return v.authSection.Layout(cgtx, th, func(ccgtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(ccgtx,
						layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
							return v.usernameEditor.Layout(cccgtx, th)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
						layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
							return v.passwordEditor.Layout(cccgtx, th)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
						layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
							return v.tokenEditor.Layout(cccgtx, th)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
						layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
							return v.nkeyEditor.Layout(cccgtx, th)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
						layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
							return v.credsEditor.Layout(cccgtx, th)
						}),
					)
				})
			}),
		)
	}
	v.editModal.CustomFocusTagsFunc = v.addModal.CustomFocusTagsFunc
	v.editModal.OnSave = func() bool {
		return v.updateContext()
	}
	v.editModal.OnClose = func() {
		v.focusRequested = true
	}

	// Set up embedded server modal
	v.embeddedServerModal.ReturnFocus = &v.listFocus
	v.embeddedServerModal.MaxWidth = unit.Dp(400)
	v.embeddedServerModal.SaveButtonText = "Connect"
	v.embeddedServerModal.CustomContent = func(gtx layout.Context, t *theme.Theme) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
				return v.jsDirEditor.Layout(cgtx, t)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
				return v.portEditor.Layout(cgtx, t)
			}),
		)
	}
	v.embeddedServerModal.CustomFocusTags = []event.Tag{
		v.jsDirEditor.FocusTag(),
		v.portEditor.FocusTag(),
	}
	v.embeddedServerModal.OnSave = func() bool {
		return v.startEmbeddedServer()
	}
	v.embeddedServerModal.OnClose = func() {
		v.focusRequested = true
	}

	return v
}

func (v *ConnectionsView) isModalVisible() bool {
	return v.addModal.Visible || v.editModal.Visible || v.removeModal.IsVisible() ||
		v.importModal.IsVisible() || v.exportModal.IsVisible() || v.embeddedServerModal.Visible
}

func (v *ConnectionsView) SetApp(app App) {
	v.app = app
}

func (v *ConnectionsView) SetNavigation(next, prev any) {
	v.next = next
	v.prev = prev
}

func (v *ConnectionsView) Info() navigator.Info {
	return navigator.Info{
		ID:    navigator.ConnectionsPageId,
		Title: "Connections",
		Icon:  icons.ActionSwapHoriz,
	}
}

func (v *ConnectionsView) OnEnter() {
	v.Refresh()
}

func (v *ConnectionsView) FirstFocusTag() any {
	return &v.listFocus
}

func (v *ConnectionsView) LastFocusTag() any {
	return &v.listFocus
}

func (v *ConnectionsView) layoutHeader(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			header := material.Label(th.Material(), unit.Sp(24), "Connection Contexts")
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

func (v *ConnectionsView) removeContext() {
	if v.app == nil {
		return
	}
	cfg := v.app.GetConfig()
	if v.selectedIdx >= 0 && v.selectedIdx < len(cfg.Contexts) {
		name := cfg.Contexts[v.selectedIdx].Name
		cfg.Contexts = append(cfg.Contexts[:v.selectedIdx], cfg.Contexts[v.selectedIdx+1:]...)
		v.selectedIdx = -1
		v.app.SaveConfig()
		if v.app != nil {
			v.app.ShowToast("Context '"+name+"' removed", components.ToastTypeSuccess)
		}
	}
}

func (v *ConnectionsView) addContext() {
	v.nameEditor.SetText("")
	v.urlEditor.SetText("")
	v.addModal.Show()
}

func (v *ConnectionsView) saveContext() bool {
	if v.app == nil {
		return false
	}
	name := v.nameEditor.GetText()
	url := v.urlEditor.GetText()
	description := v.descriptionEditor.GetText()
	username := v.usernameEditor.GetText()
	password := v.passwordEditor.GetText()
	token := v.tokenEditor.GetText()
	nkey := v.nkeyEditor.GetText()
	creds := v.credsEditor.GetText()

	if name == "" || url == "" {
		v.app.ShowToast("Please fill in all required fields", components.ToastTypeWarning)
		return false
	}

	ctx := &config.ContextInfo{
		Name:        name,
		Description: description,
		URL:         url,
		Username:    username,
		Password:    password,
		Token:       token,
		NKeyFile:    nkey,
		Credentials: creds,
		Active:      false,
	}

	cfg := v.app.GetConfig()
	cfg.Contexts = append(cfg.Contexts, ctx)
	v.app.SaveConfig()
	v.nameEditor.SetText("")
	v.urlEditor.SetText("")
	v.descriptionEditor.SetText("")
	v.usernameEditor.SetText("")
	v.passwordEditor.SetText("")
	v.tokenEditor.SetText("")
	v.nkeyEditor.SetText("")
	v.credsEditor.SetText("")

	v.app.ShowToast("Context '"+name+"' added", components.ToastTypeSuccess)
	log.Logger().Info().
		Str("name", name).
		Str("url", url).
		Str("description", description).
		Bool("has_username", username != "").
		Bool("has_password", password != "").
		Bool("has_token", token != "").
		Bool("has_nkey", nkey != "").
		Bool("has_creds", creds != "").
		Msg("Context saved")
	return true
}

func (v *ConnectionsView) importFromNatsCLI() {
	if v.app == nil {
		return
	}
	cfg := v.app.GetConfig()
	if err := cfg.ImportFromNatsCLI(); err != nil {
		v.app.ShowToast("Import failed: "+err.Error(), components.ToastTypeError)
		return
	}
	v.app.SaveConfig()
	v.app.ShowToast("Contexts imported from natscli", components.ToastTypeSuccess)
}

func (v *ConnectionsView) exportToNatsCLI() {
	if v.app == nil {
		return
	}
	cfg := v.app.GetConfig()
	if err := cfg.ExportToNatsCLI(); err != nil {
		v.app.ShowToast("Export failed: "+err.Error(), components.ToastTypeError)
		return
	}
	v.app.ShowToast("Contexts exported to natscli", components.ToastTypeSuccess)
}

func (v *ConnectionsView) editContext() {
	if v.app == nil {
		return
	}
	cfg := v.app.GetConfig()
	if v.selectedIdx >= 0 && v.selectedIdx < len(cfg.Contexts) {
		ctx := cfg.Contexts[v.selectedIdx]
		v.nameEditor.SetText(ctx.Name)
		v.descriptionEditor.SetText(ctx.Description)
		v.urlEditor.SetText(ctx.URL)
		v.usernameEditor.SetText(ctx.Username)
		v.passwordEditor.SetText(ctx.Password)
		v.tokenEditor.SetText(ctx.Token)
		v.nkeyEditor.SetText(ctx.NKeyFile)
		v.credsEditor.SetText(ctx.Credentials)
		v.editModal.Show()
	}
}

func (v *ConnectionsView) updateContext() bool {
	if v.app == nil {
		return false
	}
	name := v.nameEditor.GetText()
	url := v.urlEditor.GetText()
	description := v.descriptionEditor.GetText()
	username := v.usernameEditor.GetText()
	password := v.passwordEditor.GetText()
	token := v.tokenEditor.GetText()
	nkey := v.nkeyEditor.GetText()
	creds := v.credsEditor.GetText()

	if name == "" || url == "" {
		v.app.ShowToast("Please fill in all required fields", components.ToastTypeWarning)
		return false
	}

	cfg := v.app.GetConfig()
	if v.selectedIdx >= 0 && v.selectedIdx < len(cfg.Contexts) {
		oldName := cfg.Contexts[v.selectedIdx].Name
		cfg.Contexts[v.selectedIdx].Name = name
		cfg.Contexts[v.selectedIdx].Description = description
		cfg.Contexts[v.selectedIdx].URL = url
		cfg.Contexts[v.selectedIdx].Username = username
		cfg.Contexts[v.selectedIdx].Password = password
		cfg.Contexts[v.selectedIdx].Token = token
		cfg.Contexts[v.selectedIdx].NKeyFile = nkey
		cfg.Contexts[v.selectedIdx].Credentials = creds
		v.app.SaveConfig()

		v.app.ShowToast("Context updated", components.ToastTypeSuccess)
		log.Logger().Info().
			Str("old_name", oldName).
			Str("new_name", name).
			Str("url", url).
			Str("description", description).
			Bool("has_username", username != "").
			Bool("has_password", password != "").
			Bool("has_token", token != "").
			Bool("has_nkey", nkey != "").
			Bool("has_creds", creds != "").
			Msg("Context updated")
		return true
	}
	return false
}

func (v *ConnectionsView) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if v.focusRequested {
		v.focusRequested = false
		gtx.Execute(key.FocusCmd{Tag: &v.listFocus})
	}

	// Only handle tab navigation if no modal is visible
	if !v.isModalVisible() {
		for {
			ev, ok := gtx.Event(key.Filter{Name: key.NameTab, Optional: key.ModShift})
			if !ok {
				break
			}
			if ke, ok := ev.(key.Event); ok && ke.State == key.Press {
				v.handleTab(gtx, ke.Modifiers.Contain(key.ModShift))
			}
		}
	}

	for v.addBtn.Clicked(gtx) {
		v.addContext()
	}

	for v.refreshBtn.Clicked(gtx) {
		v.Refresh()
	}

	for v.editBtn.Clicked(gtx) {
		if v.app != nil {
			cfg := v.app.GetConfig()
			if v.selectedIdx >= 0 && v.selectedIdx < len(cfg.Contexts) {
				v.editContext()
			}
		}
	}

	for v.removeBtn.Clicked(gtx) {
		if v.app != nil {
			cfg := v.app.GetConfig()
			if v.selectedIdx >= 0 && v.selectedIdx < len(cfg.Contexts) {
				v.removeModal.Content = "Are you sure you want to remove '" + cfg.Contexts[v.selectedIdx].Name + "'?"
				v.removeModal.Show()
			}
		}
	}

	for v.connectBtn.Clicked(gtx) {
		if v.app != nil {
			cfg := v.app.GetConfig()
			if v.selectedIdx >= 0 && v.selectedIdx < len(cfg.Contexts) {
				ctx := cfg.Contexts[v.selectedIdx]
				if ctx.Connected {
					v.disconnect()
				} else {
					v.connect()
				}
			}
		}
	}

	for v.importBtn.Clicked(gtx) {
		v.importModal.Show()
	}

	for v.exportBtn.Clicked(gtx) {
		v.exportModal.Show()
	}

	for v.switchBtn.Clicked(gtx) {
		v.showContextSwitcher()
	}

	for v.prevCtxBtn.Clicked(gtx) {
		v.SwitchToPreviousContext()
	}

	for v.embeddedServerBtn.Clicked(gtx) {
		if v.embeddedRunning {
			v.stopEmbeddedServer()
		} else {
			v.embeddedServerModal.Show()
		}
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
					layout.Rigid(layout.Spacer{Height: unit.Dp(24)}.Layout),
					layout.Flexed(1, func(cccgtx layout.Context) layout.Dimensions {
						return v.layoutContent(cccgtx, th)
					}),
				)
			})
		}),
		layout.Stacked(func(cgtx layout.Context) layout.Dimensions {
			return v.layoutModals(cgtx, th)
		}),
		layout.Stacked(func(cgtx layout.Context) layout.Dimensions {
			return v.loadingOverlay.Layout(cgtx, th)
		}),
	)
}

func (v *ConnectionsView) handleTab(gtx layout.Context, shift bool) {
	tags := []any{
		&v.refreshBtn,
		&v.importBtn,
		&v.exportBtn,
		&v.addBtn,
	}

	isSelected := false
	hasContexts := false
	if v.app != nil {
		cfg := v.app.GetConfig()
		hasContexts = len(cfg.Contexts) > 0
		isSelected = v.selectedIdx >= 0 && v.selectedIdx < len(cfg.Contexts)
	}

	if isSelected {
		tags = append(tags, &v.connectBtn, &v.editBtn, &v.removeBtn)
	}

	if hasContexts {
		tags = append(tags, &v.listFocus)
	}

	HandleTab(gtx, shift, tags, v.next, v.prev)
}

func (v *ConnectionsView) layoutModals(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	return layout.Stack{}.Layout(gtx,
		layout.Stacked(func(cgtx layout.Context) layout.Dimensions {
			return v.addModal.Layout(cgtx, th)
		}),
		layout.Stacked(func(cgtx layout.Context) layout.Dimensions {
			return v.editModal.Layout(cgtx, th)
		}),
		layout.Stacked(func(cgtx layout.Context) layout.Dimensions {
			if v.removeModal.IsVisible() {
				return v.removeModal.Layout(cgtx, th)
			}
			return layout.Dimensions{}
		}),
		layout.Stacked(func(cgtx layout.Context) layout.Dimensions {
			if v.importModal.IsVisible() {
				return v.importModal.Layout(cgtx, th)
			}
			return layout.Dimensions{}
		}),
		layout.Stacked(func(cgtx layout.Context) layout.Dimensions {
			if v.exportModal.IsVisible() {
				return v.exportModal.Layout(cgtx, th)
			}
			return layout.Dimensions{}
		}),
		layout.Stacked(func(cgtx layout.Context) layout.Dimensions {
			return v.embeddedServerModal.Layout(cgtx, th)
		}),
	)
}

func (v *ConnectionsView) layoutActions(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	addBtn := components.Button(th, &v.addBtn, icons.ContentAddCircle, components.IconPositionStart, "Add Context")
	editBtn := components.SecondaryButton(th, &v.editBtn, icons.EditorModeEdit, components.IconPositionStart, "Edit")
	removeBtn := components.SecondaryButton(th, &v.removeBtn, icons.ActionDelete, components.IconPositionStart, "Remove")
	connectBtn := components.Button(th, &v.connectBtn, icons.AVPlayArrow, components.IconPositionStart, "Connect")
	refreshBtn := components.SecondaryButton(th, &v.refreshBtn, icons.NavigationRefresh, components.IconPositionStart, "Refresh")
	importBtn := components.SecondaryButton(th, &v.importBtn, icons.FileFileDownload, components.IconPositionStart, "Import")
	exportBtn := components.SecondaryButton(th, &v.exportBtn, icons.FileFileUpload, components.IconPositionStart, "Export")
	embeddedBtn := components.Button(th, &v.embeddedServerBtn, icons.DeviceStorage, components.IconPositionStart, "Embedded Server")
	if v.embeddedRunning {
		embeddedBtn.Text = "Stop Embedded"
		embeddedBtn.Icon = icons.NavigationClose
	}

	isSelected := false
	if v.app != nil {
		cfg := v.app.GetConfig()
		isSelected = v.selectedIdx >= 0 && v.selectedIdx < len(cfg.Contexts)
		if isSelected {
			if cfg.Contexts[v.selectedIdx].Connected {
				connectBtn.Text = "Disconnect"
				connectBtn.Icon = icons.NavigationClose
			}
		}
	}

	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceEnd}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return refreshBtn.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return importBtn.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return exportBtn.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return addBtn.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return embeddedBtn.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceEnd}.Layout(cgtx,
				layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
					if !isSelected {
						ccgtx = ccgtx.Disabled()
					}
					// Fixed width to prevent jumping when text changes
					ccgtx.Constraints.Min.X = gtx.Dp(unit.Dp(120))
					ccgtx.Constraints.Max.X = gtx.Dp(unit.Dp(120))
					return connectBtn.Layout(ccgtx, th)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
					if !isSelected {
						ccgtx = ccgtx.Disabled()
					}
					return editBtn.Layout(ccgtx, th)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
					if !isSelected {
						ccgtx = ccgtx.Disabled()
					}
					return removeBtn.Layout(ccgtx, th)
				}),
			)
		}),
	)
}

func (v *ConnectionsView) layoutContent(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if v.app == nil {
		return layout.Dimensions{}
	}
	cfg := v.app.GetConfig()
	if len(cfg.Contexts) == 0 {
		return v.layoutEmptyState(gtx, th)
	}
	return v.split.Layout(gtx, th,
		func(cgtx layout.Context) layout.Dimensions {
			return v.layoutContextsList(cgtx, th)
		},
		func(cgtx layout.Context) layout.Dimensions {
			return v.layoutContextDetails(cgtx, th)
		},
	)
}

func (v *ConnectionsView) layoutEmptyState(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	emptyState := components.EmptyState{
		Icon:     icons.ActionSwapHoriz,
		Title:    "No Connection Contexts",
		Message:  "Configure your NATS connection contexts to get started.\nUse the 'Add Context' button to create a new connection.",
		Button:   &v.addBtn,
		BtnText:  "Add Context",
		OnAction: func() { v.addModal.Show() },
	}
	return emptyState.Layout(gtx, th)
}

func (v *ConnectionsView) layoutContextsList(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	return v.layoutListItems(gtx, th)
}

func (v *ConnectionsView) layoutListItems(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if v.app == nil {
		return layout.Dimensions{}
	}
	cfg := v.app.GetConfig()
	contexts := cfg.Contexts

	for len(v.clickableList) < len(contexts) {
		v.clickableList = append(v.clickableList, widget.Clickable{})
	}

	// Register focus tag
	area := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
	event.Op(gtx.Ops, &v.listFocus)
	area.Pop()

	// Handle keyboard navigation for the list (only when no modal is open)
	if !v.isModalVisible() {
		for {
			e, ok := gtx.Event(
				key.Filter{Focus: &v.listFocus, Name: key.NameUpArrow},
				key.Filter{Focus: &v.listFocus, Name: key.NameDownArrow},
				key.Filter{Focus: &v.listFocus, Name: key.NameReturn, Optional: key.ModShift},
				key.Filter{Focus: &v.listFocus, Name: key.NameEnter, Optional: key.ModShift},
				key.Filter{Focus: &v.listFocus, Name: key.NameDeleteForward},
				key.Filter{Focus: &v.listFocus, Name: key.NameDeleteBackward},
			)
			if !ok {
				break
			}
			if ke, ok := e.(key.Event); ok && ke.State == key.Press {
				switch ke.Name {
				case key.NameUpArrow:
					if v.selectedIdx > 0 {
						v.selectedIdx--
						v.listState.ScrollTo(v.selectedIdx)
					}
				case key.NameDownArrow:
					if v.selectedIdx < len(contexts)-1 {
						v.selectedIdx++
						v.listState.ScrollTo(v.selectedIdx)
					}
				case key.NameReturn, key.NameEnter:
					if v.selectedIdx >= 0 && v.selectedIdx < len(contexts) {
						if ke.Modifiers.Contain(key.ModShift) {
							v.editContext()
						} else {
							ctx := contexts[v.selectedIdx]
							if ctx.Connected {
								v.disconnect()
							} else {
								v.connect()
							}
						}
					}
				case key.NameDeleteForward, key.NameDeleteBackward:
					if v.selectedIdx >= 0 && v.selectedIdx < len(contexts) {
						v.removeModal.Content = "Are you sure you want to remove '" + contexts[v.selectedIdx].Name + "'?"
						v.removeModal.Show()
					}
				}
				gtx.Execute(op.InvalidateCmd{})
			}
		}
	}

	for v.listFocus.Clicked(gtx) {
		gtx.Execute(key.FocusCmd{Tag: &v.listFocus})
	}

	dims := v.listFocus.Layout(gtx, func(cgtx layout.Context) layout.Dimensions {
		return layout.Stack{}.Layout(cgtx,
			layout.Expanded(func(ccgtx layout.Context) layout.Dimensions {
				if ccgtx.Source.Focused(&v.listFocus) {
					components.DrawFocusRing(ccgtx, th.BorderColorFocused, ccgtx.Constraints.Max, 4)
				}
				return layout.Dimensions{Size: ccgtx.Constraints.Max}
			}),
			layout.Stacked(func(ccgtx layout.Context) layout.Dimensions {
				ccgtx.Constraints.Min.X = ccgtx.Constraints.Max.X
				return material.List(th.Material(), v.listState).Layout(ccgtx, len(contexts), func(cccgtx layout.Context, index int) layout.Dimensions {
					for {
						ev, ok := v.clickableList[index].Update(cccgtx)
						if !ok {
							break
						}
						v.selectedIdx = index
						gtx.Execute(key.FocusCmd{Tag: &v.listFocus})
						cccgtx.Execute(op.InvalidateCmd{})

						if ev.NumClicks == 2 {
							if ev.Modifiers.Contain(key.ModShift) {
								v.editContext()
							} else {
								ctx := contexts[index]
								if ctx.Connected {
									v.disconnect()
								} else {
									v.connect()
								}
							}
						}
					}

					ctx := contexts[index]
					selected := index == v.selectedIdx

					return v.clickableList[index].Layout(cccgtx, func(ccccgtx layout.Context) layout.Dimensions {
						if selected {
							bg := th.ActionButtonBgColor
							bg.A = 32
							paint.FillShape(ccccgtx.Ops, bg, clip.Rect{Max: ccccgtx.Constraints.Max}.Op())
						}

						return layout.UniformInset(unit.Dp(12)).Layout(ccccgtx, func(cccccgtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Vertical}.Layout(cccccgtx,
								layout.Rigid(func(c6gtx layout.Context) layout.Dimensions {
									lbl := material.Label(th.Material(), unit.Sp(14), ctx.Name)
									lbl.Color = th.TextColor
									lbl.Font.Weight = font.Bold
									return lbl.Layout(c6gtx)
								}),
								layout.Rigid(func(c6gtx layout.Context) layout.Dimensions {
									displayText := ctx.URL
									if ctx.Description != "" {
										displayText = ctx.Description
									}
									lbl := material.Label(th.Material(), unit.Sp(12), displayText)
									lbl.Color = th.SecondaryTextColor
									return lbl.Layout(c6gtx)
								}),
							)
						})
					})
				})
			}),
		)
	})

	if gtx.Focused(&v.listFocus) {
		components.DrawFocusRing(gtx, th.BorderColorFocused, dims.Size, gtx.Dp(unit.Dp(4)))
	}

	return dims
}

func (v *ConnectionsView) connect() {
	if v.app == nil {
		return
	}
	cfg := v.app.GetConfig()
	if v.selectedIdx < 0 || v.selectedIdx >= len(cfg.Contexts) {
		return
	}

	ctx := cfg.Contexts[v.selectedIdx]
	ctx.Active = true

	v.loadingOverlay.Loading = true
	v.loadingOverlay.Message = "Connecting to " + ctx.Name + "..."

	go func() {
		defer func() {
			v.loadingOverlay.Loading = false
		}()

		// Stop embedded server if running
		if v.embeddedServer != nil {
			if v.natsClient != nil {
				v.natsClient.Close()
				v.natsClient = nil
			}
			port := v.embeddedServer.Port()
			v.embeddedServer.Stop()
			v.embeddedServer = nil
			v.embeddedRunning = false
			v.embeddedConnected = false

			log.Logger().Info().
				Int("port", port).
				Msg("Stopped embedded NATS server (connecting to external server)")
		}

		configReq := nats.ConnectionConfig{
			URL:      ctx.URL,
			Username: ctx.Username,
			Password: ctx.Password,
			Token:    ctx.Token,
			NKeyFile: ctx.NKeyFile,
			Creds:    ctx.Credentials,
		}

		client, err := nats.NewClient(configReq)
		if err != nil {
			if v.app != nil {
				v.app.ShowToast("Connection failed: "+err.Error(), components.ToastTypeError)
			}
			return
		}

		if v.natsClient != nil {
			v.natsClient.Close()
		}

		v.natsClient = client
		ctx.Connected = true

		for i, c := range cfg.Contexts {
			if i != v.selectedIdx {
				c.Connected = false
			}
		}

		if v.app != nil {
			v.app.SetStatus("Connected", true)
			v.app.SetContextName(ctx.Name)
			v.app.UpdateStatusText("Ready")
			v.app.ShowToast("Connected to "+ctx.Name, components.ToastTypeSuccess)

			// Log connection with context
			log.Logger().Info().
				Str("context", ctx.Name).
				Str("url", ctx.URL).
				Str("server_id", client.ConnectedServerID()).
				Str("server_name", client.ConnectedServerName()).
				Str("server_addr", client.ConnectedAddr()).
				Str("server_version", client.ConnectedServerVersion()).
				Msg("Connected to NATS server")
		}
	}()
}

func (v *ConnectionsView) disconnect() {
	if v.app == nil {
		return
	}
	cfg := v.app.GetConfig()
	if v.selectedIdx < 0 || v.selectedIdx >= len(cfg.Contexts) {
		return
	}

	ctx := cfg.Contexts[v.selectedIdx]

	if v.natsClient != nil {
		v.natsClient.Close()
		v.natsClient = nil
	}

	ctx.Connected = false

	if v.app != nil {
		v.app.SetStatus("Disconnected", false)
		v.app.SetContextName("")
		v.app.UpdateStatusText("Disconnected")
		v.app.ShowToast("Disconnected from "+ctx.Name, components.ToastTypeInfo)

		// Log disconnection with context
		log.Logger().Info().
			Str("context", ctx.Name).
			Str("url", ctx.URL).
			Msg("Disconnected from NATS server")
	}
}

func (v *ConnectionsView) startEmbeddedServer() bool {
	if v.app == nil {
		return false
	}

	storeDir := v.jsDirEditor.GetText()
	portStr := v.portEditor.GetText()

	if storeDir == "" {
		storeDir = "./"
	}
	if portStr == "" {
		portStr = "4222"
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		v.app.ShowToast("Invalid port number", components.ToastTypeError)
		return false
	}

	// Disconnect from current connection if any
	if v.natsClient != nil {
		v.natsClient.Close()
		v.natsClient = nil
	}

	// Disconnect any connected context
	cfg := v.app.GetConfig()
	for _, ctx := range cfg.Contexts {
		if ctx.Connected {
			ctx.Connected = false
		}
	}

	v.loadingOverlay.Loading = true
	v.loadingOverlay.Message = "Starting embedded NATS server..."

	go func() {
		defer func() {
			v.loadingOverlay.Loading = false
		}()

		c := embedded.Config{
			Port:       port,
			StoreDir:   storeDir,
			ServerName: "nats-desktop-embedded",
		}

		srv, err := embedded.New(c)
		if err != nil {
			if v.app != nil {
				v.app.ShowToast("Failed to create embedded server: "+err.Error(), components.ToastTypeError)
			}
			return
		}

		if err := srv.Start(); err != nil {
			if v.app != nil {
				v.app.ShowToast("Failed to start embedded server: "+err.Error(), components.ToastTypeError)
			}
			return
		}

		// Connect to the embedded server
		clientURL := srv.ClientURL()
		client, err := nats.NewClient(nats.ConnectionConfig{
			URL: clientURL,
		})
		if err != nil {
			srv.Stop()
			if v.app != nil {
				v.app.ShowToast("Failed to connect to embedded server: "+err.Error(), components.ToastTypeError)
			}
			return
		}

		v.embeddedServer = srv
		v.embeddedRunning = true
		v.embeddedConnected = true
		v.natsClient = client

		if v.app != nil {
			v.app.SetStatus("Connected", true)
			v.app.SetContextName("embedded server")
			v.app.UpdateStatusText("Ready")
			v.app.ShowToast("Connected to embedded NATS server on port "+strconv.Itoa(port), components.ToastTypeSuccess)

			log.Logger().Info().
				Int("port", port).
				Str("store_dir", storeDir).
				Str("url", clientURL).
				Msg("Started and connected to embedded NATS server")
		}
	}()

	return true
}

func (v *ConnectionsView) stopEmbeddedServer() {
	if v.app == nil {
		return
	}

	if v.embeddedServer != nil {
		v.loadingOverlay.Loading = true
		v.loadingOverlay.Message = "Stopping embedded NATS server..."

		go func() {
			defer func() {
				v.loadingOverlay.Loading = false
			}()

			if v.natsClient != nil {
				v.natsClient.Close()
				v.natsClient = nil
			}

			v.embeddedServer.Stop()
			port := v.embeddedServer.Port()
			v.embeddedServer = nil
			v.embeddedRunning = false
			v.embeddedConnected = false

			if v.app != nil {
				v.app.SetStatus("Disconnected", false)
				v.app.SetContextName("")
				v.app.UpdateStatusText("Disconnected")
				v.app.ShowToast("Stopped embedded NATS server (port "+strconv.Itoa(port)+")", components.ToastTypeInfo)

				log.Logger().Info().
					Int("port", port).
					Msg("Stopped embedded NATS server")
			}
		}()
	}
}

func (v *ConnectionsView) layoutContextDetails(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if v.app == nil {
		return layout.Dimensions{}
	}
	cfg := v.app.GetConfig()
	if v.selectedIdx < 0 || v.selectedIdx >= len(cfg.Contexts) {
		return layout.Center.Layout(gtx, func(cgtx layout.Context) layout.Dimensions {
			lbl := material.Label(th.Material(), unit.Sp(14), "Select a connection to see details")
			lbl.Color = th.TextColor
			return lbl.Layout(cgtx)
		})
	}

	ctx := cfg.Contexts[v.selectedIdx]
	return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(cgtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(cgtx,
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				header := material.Label(th.Material(), unit.Sp(18), "Connection Context: "+ctx.Name)
				header.Color = th.TextColor
				header.Font.Weight = font.Bold
				return header.Layout(ccgtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				if ctx.Description != "" {
					return layoutDetailRow(ccgtx, th, "Description", ctx.Description)
				}
				return layout.Dimensions{}
			}),
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return layoutDetailRow(ccgtx, th, "URL", ctx.URL)
			}),
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				authType := "None"
				switch {
				case ctx.Username != "":
					authType = "User/Pass"
				case ctx.Token != "":
					authType = "Token"
				case ctx.NKeyFile != "":
					authType = "NKey"
				case ctx.Credentials != "":
					authType = "Credentials"
				}
				return layoutDetailRow(ccgtx, th, "Auth Type", authType)
			}),
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				statusText := "Disconnected"
				statusColor := th.TextColor
				if ctx.Connected {
					statusText = "Connected"
					statusColor = theme.LightGreen
				}
				return layoutDetailRowColored(ccgtx, th, "Status", statusText, statusColor)
			}),
		)
	})
}

func (v *ConnectionsView) SwitchToPreviousContext() {
	if v.app == nil {
		return
	}
	cfg := v.app.GetConfig()
	if cfg.SwitchToPreviousContext() {
		v.app.SaveConfig()
		// Find and select the previous context
		for i, ctx := range cfg.Contexts {
			if ctx.Active {
				v.selectedIdx = i
				v.app.ShowToast("Switched to previous context: "+ctx.Name, components.ToastTypeSuccess)
				break
			}
		}
	} else {
		v.app.ShowToast("No previous context available", components.ToastTypeWarning)
	}
}

func (v *ConnectionsView) showContextSwitcher() {
	// This will be implemented with a dropdown or modal for context switching
	// For now, just cycle through contexts
	if v.app == nil {
		return
	}
	cfg := v.app.GetConfig()
	if len(cfg.Contexts) == 0 {
		v.app.ShowToast("No contexts available", components.ToastTypeWarning)
		return
	}

	// Simple cycling for now
	v.selectedIdx = (v.selectedIdx + 1) % len(cfg.Contexts)
	ctx := cfg.Contexts[v.selectedIdx]
	v.app.ShowToast("Selected context: "+ctx.Name, components.ToastTypeInfo)
}

// HandleShortcuts processes keyboard shortcuts for the connections view
// Returns true if a shortcut was handled
func (v *ConnectionsView) HandleShortcuts(gtx layout.Context) bool {
	// Check if any modal is visible first - view shortcuts disabled when modal is open
	if v.isModalVisible() {
		return false
	}

	// View-specific shortcuts - process only ONE event per frame
	// Order matters: check more specific shortcuts (with Shift) before general ones
	ev, ok := gtx.Event(
		key.Filter{Name: key.Name("E"), Optional: key.ModShortcut | key.ModShift},
		key.Filter{Name: key.Name("E"), Optional: key.ModShortcut},
		key.Filter{Name: key.Name("I"), Optional: key.ModShortcut},
		key.Filter{Name: key.NameReturn},
		key.Filter{Name: key.NameEnter},
		key.Filter{Name: key.Name("R"), Optional: key.ModShortcut},
		key.Filter{Name: key.Name("N"), Optional: key.ModShortcut},
		key.Filter{Name: key.NameDeleteForward},
		key.Filter{Name: key.NameDeleteBackward},
		key.Filter{Name: key.Name("C"), Optional: key.ModCtrl | key.ModShift},
	)
	if !ok {
		return false
	}

	if ke, ok := ev.(key.Event); ok && ke.State == key.Press {
		switch {
		case ke.Name == key.Name("E") && ke.Modifiers.Contain(key.ModShortcut|key.ModShift):
			// Ctrl+Shift+E = Export (check this FIRST before Ctrl+E)
			v.exportBtn.Click()
			return true
		case ke.Name == key.Name("E") && ke.Modifiers.Contain(key.ModShortcut):
			// Ctrl+E = Embedded Server
			if v.embeddedRunning {
				v.stopEmbeddedServer()
			} else {
				v.embeddedServerBtn.Click()
			}
			return true
		case ke.Name == key.Name("I") && ke.Modifiers.Contain(key.ModShortcut):
			v.importBtn.Click()
			return true
		case ke.Name == key.Name("R") && ke.Modifiers.Contain(key.ModShortcut):
			v.refreshBtn.Click()
			return true
		case ke.Name == key.Name("N") && ke.Modifiers.Contain(key.ModShortcut):
			v.addBtn.Click()
			return true
		case ke.Name == key.NameDeleteForward || ke.Name == key.NameDeleteBackward:
			if v.selectedIdx >= 0 && v.app != nil && v.selectedIdx < len(v.app.GetConfig().Contexts) {
				v.removeBtn.Click()
				return true
			}
		case ke.Name == key.NameReturn || ke.Name == key.NameEnter:
			if v.selectedIdx >= 0 && v.app != nil {
				v.connectBtn.Click()
				return true
			}
		case ke.Name == key.Name("C") && ke.Modifiers.Contain(key.ModCtrl|key.ModShift):
			if v.selectedIdx >= 0 && v.app != nil && v.selectedIdx < len(v.app.GetConfig().Contexts) {
				ctx := v.app.GetConfig().Contexts[v.selectedIdx]
				tsv := fmt.Sprintf("%s\t%s\t%s\t%s", ctx.Name, ctx.URL, ctx.Description, ctx.Username)
				components.CopyToClipboard(gtx, tsv)
				v.app.ShowToast("Copied: "+components.TruncateText(tsv, 50), components.ToastTypeSuccess)
				return true
			}
		case ke.Name == key.Name("C") && ke.Modifiers.Contain(key.ModCtrl):
			if v.selectedIdx >= 0 && v.app != nil && v.selectedIdx < len(v.app.GetConfig().Contexts) {
				ctx := v.app.GetConfig().Contexts[v.selectedIdx]
				components.CopyToClipboard(gtx, ctx.Name)
				v.app.ShowToast("Copied: "+ctx.Name, components.ToastTypeSuccess)
				return true
			}
		}
	}
	return false
}

// GetShortcutsHelp returns help text for this view shortcuts
func (v *ConnectionsView) GetShortcutsHelp() []shortcuts.Shortcut {
	return []shortcuts.Shortcut{
		shortcuts.Refresh(func() {}),
		shortcuts.Create(func() {}),
		shortcuts.Delete(func() bool { return v.selectedIdx >= 0 }, func() {}),
		shortcuts.Custom("Embedded Server", "Start/stop embedded NATS server", key.Name("E"), key.ModShortcut, nil, func() {}),
		shortcuts.Import(func() {}),
		shortcuts.Export(func() {}),
		shortcuts.Connect(func() bool { return v.selectedIdx >= 0 }, func() {}),
		shortcuts.CopyName(func() bool { return v.selectedIdx >= 0 }, func() {}),
		shortcuts.CopyRow(func() bool { return v.selectedIdx >= 0 }, func() {}),
	}
}
