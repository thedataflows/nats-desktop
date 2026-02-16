package components

import (
	"image"
	"image/color"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/thedataflows/nats-desktop/internal/ui/theme"
)

// Prompt is a modal dialog that prompts the user for a response.
const (
	ModalTypeInfo = "info"
	ModalTypeWarn = "warn"
	ModalTypeErr  = "err"
)

var (
	colors = map[string]color.NRGBA{
		// Red
		ModalTypeErr: {R: 0xD1, G: 0x1E, B: 0x35, A: 0xFF},
		// Light blue
		ModalTypeInfo: {R: 0x1D, G: 0xBF, B: 0xEC, A: 0xFF},
		// Yellow
		ModalTypeWarn: {R: 0xFD, G: 0xB5, B: 0x0E, A: 0xFF},
	}
)

type Prompt struct {
	Title   string
	Content string
	Type    string
	Visible bool

	ReturnFocus event.Tag

	Backdrop widget.Clickable

	// contentClickable captures clicks on the modal content to prevent
	// them from propagating to the backdrop
	contentClickable widget.Clickable

	rememberBool *widget.Bool

	options []Option
	result  string

	onSubmit func(selectedOption string, remember bool)
	onClose  func()
}

type Option struct {
	Text   string
	Button widget.Clickable
	Icon   *widget.Icon
}

func NewPrompt(title, content, modalType string, options ...Option) *Prompt {
	return &Prompt{
		Title:   title,
		Content: content,
		Type:    modalType,
		options: options,
	}
}

func (p *Prompt) SetOptions(options ...Option) {
	p.options = options
}

func (p *Prompt) Show() {
	p.Visible = true
}

func (p *Prompt) Hide() {
	p.Visible = false
}

func (p *Prompt) IsVisible() bool {
	return p.Visible
}

func (p *Prompt) SetOnClose(f func()) {
	p.onClose = f
}

func (p *Prompt) closePrompt(gtx layout.Context) {
	p.Visible = false
	if p.ReturnFocus != nil {
		gtx.Execute(key.FocusCmd{Tag: p.ReturnFocus})
	}
	if p.onClose != nil {
		p.onClose()
	}
	gtx.Execute(op.InvalidateCmd{})
}

func (p *Prompt) WithRememberBool() {
	p.rememberBool = &widget.Bool{Value: false}
}

func (p *Prompt) WithoutRememberBool() {
	p.rememberBool = nil
}

func (p *Prompt) SetOnSubmit(f func(selectedOption string, remember bool)) {
	p.onSubmit = f
}

func (p *Prompt) submit() {
	if p.onSubmit == nil {
		return
	}

	if !p.Visible {
		return
	}

	if p.rememberBool == nil {
		p.onSubmit(p.result, false)
		return
	}

	p.onSubmit(p.result, p.rememberBool.Value)
}

func (p *Prompt) handleTab(gtx layout.Context, shift bool) {
	var tags []event.Tag
	if p.rememberBool != nil {
		tags = append(tags, p.rememberBool)
	}
	for i := range p.options {
		tags = append(tags, &p.options[i].Button)
	}

	if len(tags) == 0 {
		return
	}

	curIdx := -1
	for i, tag := range tags {
		if gtx.Source.Focused(tag) {
			curIdx = i
			break
		}
	}

	if curIdx == -1 {
		gtx.Execute(key.FocusCmd{Tag: tags[0]})
		return
	}

	var nextIdx int
	if shift {
		nextIdx = (curIdx - 1 + len(tags)) % len(tags)
	} else {
		nextIdx = (curIdx + 1) % len(tags)
	}
	gtx.Execute(key.FocusCmd{Tag: tags[nextIdx]})
}

func (p *Prompt) Result() (string, bool) {
	if p.result == "" {
		return "", false
	}

	if p.rememberBool != nil {
		return p.result, p.rememberBool.Value
	}

	return p.result, false
}

func (p *Prompt) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if !p.Visible {
		return layout.Dimensions{}
	}

	// Capture all key events within the modal area
	area := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
	event.Op(gtx.Ops, p)
	area.Pop()

	// Handle Keyboard Events
	for {
		ev, ok := gtx.Event(
			key.Filter{Name: key.NameEscape},
			key.Filter{Name: key.NameTab, Optional: key.ModShift},
			key.Filter{Name: key.NameReturn},
			key.Filter{Name: key.NameEnter},
			key.Filter{Name: key.NameSpace},
		)
		if !ok {
			break
		}
		if ke, ok := ev.(key.Event); ok && ke.State == key.Press {
			switch ke.Name {
			case key.NameEscape:
				p.closePrompt(gtx)
			case key.NameTab:
				p.handleTab(gtx, ke.Modifiers.Contain(key.ModShift))
			case key.NameReturn, key.NameEnter, key.NameSpace:
				for i := range p.options {
					if gtx.Source.Focused(&p.options[i].Button) {
						p.result = p.options[i].Text
						p.submit()
						p.closePrompt(gtx)
						break
					}
				}
			}
		}
	}

	// Initial focus if nothing is focused
	anyFocused := false
	for i := range p.options {
		if gtx.Source.Focused(&p.options[i].Button) {
			anyFocused = true
			break
		}
	}
	if !anyFocused && len(p.options) > 0 {
		gtx.Execute(key.FocusCmd{Tag: &p.options[0].Button})
	}

	textColor := th.Palette.ContrastFg
	switch p.Type {
	case ModalTypeErr:
		textColor = color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
	case ModalTypeInfo:
		textColor = color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0xFF}
	case ModalTypeWarn:
		textColor = color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0xFF}
	}

	return layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Expanded(func(cgtx layout.Context) layout.Dimensions {
			// Draw semi-transparent backdrop - use higher opacity when on top of another modal
			bg := color.NRGBA{A: 220}
			paint.FillShape(cgtx.Ops, bg, clip.Rect{Max: cgtx.Constraints.Max}.Op())

			// Handle backdrop clicks
			return p.Backdrop.Layout(cgtx, func(ccgtx layout.Context) layout.Dimensions {
				if p.Backdrop.Clicked(ccgtx) {
					p.closePrompt(ccgtx)
				}
				return layout.Dimensions{Size: ccgtx.Constraints.Max}
			})
		}),
		layout.Stacked(func(cgtx layout.Context) layout.Dimensions {
			cgtx.Constraints.Max.X = cgtx.Dp(unit.Dp(500))
			cgtx.Constraints.Min.X = cgtx.Constraints.Max.X

			// Wrap content in a clickable to capture clicks and prevent them
			// from propagating to the backdrop
			return p.contentClickable.Layout(cgtx, func(ccgtx layout.Context) layout.Dimensions {
				return layout.Background{}.Layout(ccgtx,
					func(ccgtx layout.Context) layout.Dimensions {
						defer clip.UniformRRect(image.Rectangle{Max: ccgtx.Constraints.Min}, 4).Push(ccgtx.Ops).Pop()
						paint.Fill(ccgtx.Ops, colors[p.Type])
						return layout.Dimensions{Size: ccgtx.Constraints.Min}
					}, func(ccgtx layout.Context) layout.Dimensions {
						return layout.UniformInset(unit.Dp(24)).Layout(ccgtx, func(cccgtx layout.Context) layout.Dimensions {
							return layout.Flex{
								Axis:      layout.Vertical,
								Alignment: layout.Start,
							}.Layout(cccgtx,
								layout.Rigid(func(c4gtx layout.Context) layout.Dimensions {
									return layout.Inset{Bottom: unit.Dp(12)}.Layout(c4gtx, func(c5gtx layout.Context) layout.Dimensions {
										// Use explicit black color for title to ensure consistency
										titleColor := color.NRGBA{R: 0, G: 0, B: 0, A: 255}
										if p.Type == ModalTypeErr {
											titleColor = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
										}
										h := material.H6(th.Material(), p.Title)
										h.Color = titleColor
										return h.Layout(c5gtx)
									})
								}),
								layout.Rigid(func(c4gtx layout.Context) layout.Dimensions {
									return layout.Inset{Bottom: unit.Dp(24)}.Layout(c4gtx, func(c5gtx layout.Context) layout.Dimensions {
										// Use explicit color for content to ensure consistency
										// Warning modals use yellow background, so use black text
										// Error modals use red background, so use white text
										contentColor := color.NRGBA{R: 0, G: 0, B: 0, A: 255}
										if p.Type == ModalTypeErr {
											contentColor = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
										}
										// Match modal.go pattern exactly
										c5gtx.Constraints.Min.X = c5gtx.Constraints.Max.X
										lbl := material.Label(th.Material(), unit.Sp(14), p.Content)
										lbl.Color = contentColor
										return lbl.Layout(c5gtx)
									})
								}),
								layout.Rigid(func(c4gtx layout.Context) layout.Dimensions {
									count := len(p.options)
									if p.rememberBool != nil {
										count++
									}

									items := make([]layout.FlexChild, 0, count)
									if p.rememberBool != nil {
										items = append(
											items,
											layout.Rigid(func(c5gtx layout.Context) layout.Dimensions {
												cb := CheckBox(th.Material(), p.rememberBool, "Don't ask again")
												cb.SetTheme(th)
												cb.Color = textColor
												cb.IconColor = textColor
												return cb.Layout(c5gtx)
											}),
											layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
										)
									}

									for i := range p.options {
										idx := i

										if p.options[idx].Button.Clicked(c4gtx) {
											p.result = p.options[idx].Text
											p.submit()
											p.closePrompt(c4gtx)
										}

										items = append(
											items,
											layout.Rigid(func(c5gtx layout.Context) layout.Dimensions {
												btn := Button(th, &p.options[idx].Button, nil, IconPositionStart, p.options[idx].Text)
												btn.Background = theme.White
												btn.Color = theme.Black
												return btn.Layout(c5gtx, th)
											}),
											layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
										)
									}

									return layout.Flex{
										Axis:      layout.Horizontal,
										Alignment: layout.Middle,
										Spacing:   layout.SpaceStart,
									}.Layout(cccgtx, items...)
								}),
							)
						})
					},
				)
			})
		}),
	)
}

// HandleShortcuts processes keyboard shortcuts for the prompt
// Returns true if a shortcut was handled
// This should be called by views to let prompts handle their own shortcuts before view shortcuts
func (p *Prompt) HandleShortcuts(gtx layout.Context) bool {
	if !p.Visible {
		return false
	}

	for {
		ev, ok := gtx.Event(
			key.Filter{Name: key.NameEscape},
		)
		if !ok {
			break
		}
		if ke, ok := ev.(key.Event); ok && ke.State == key.Press {
			if ke.Name == key.NameEscape {
				p.Hide()
				if p.onClose != nil {
					p.onClose()
				}
				return true
			}
		}
	}
	return false
}
