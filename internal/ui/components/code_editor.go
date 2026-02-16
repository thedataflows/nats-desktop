package components

import (
	"encoding/json"
	"fmt"
	"image/color"
	"io"
	"strings"

	"gioui.org/font"
	"gioui.org/io/clipboard"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/oligo/gvcode"
	gvcolor "github.com/oligo/gvcode/color"
	"github.com/oligo/gvcode/textstyle/syntax"
	gvwidget "github.com/oligo/gvcode/widget"
	"github.com/tidwall/pretty"

	"github.com/thedataflows/nats-desktop/internal/ui/theme"
)

const (
	CodeLanguageJSON = "JSON"
	CodeLanguageXML  = "XML"
	CodeLanguageText = "Plain Text"
)

type CodeEditor struct {
	editor *gvcode.Editor
	code   string // beautified/displayed code
	raw    string // original raw code

	styledCode string
	tokens     []syntax.Token

	lexer chroma.Lexer
	lang  string

	focused      bool
	readOnly     bool
	requestFocus bool

	onChange func(text string)
	onTab    func(gtx layout.Context, shift bool)

	border widget.Border

	copyBtn widget.Clickable

	xScroll widget.Scrollbar
	yScroll widget.Scrollbar
}

type colorStyle struct {
	scope     syntax.StyleScope
	textStyle syntax.TextStyle
	color     gvcolor.Color
	bg        gvcolor.Color
}

var registry = make(map[string][]colorStyle)

func NewCodeEditor(code string, lang string, th *theme.Theme) *CodeEditor {
	c := &CodeEditor{
		code:     code,
		lang:     lang,
		readOnly: true,
	}

	c.editor = gvwidget.NewEditor(th.Material())

	c.lexer = getLexer(lang)
	c.SetTheme(th)

	c.border = widget.Border{
		Color:        th.BorderColor,
		Width:        unit.Dp(1),
		CornerRadius: unit.Dp(8),
	}

	c.SetCode(code)
	return c
}

func (c *CodeEditor) SetTheme(th *theme.Theme) {
	colorScheme := syntax.ColorScheme{}
	colorScheme.SelectColor = gvcolor.MakeColor(th.TextSelectionColor).MulAlpha(0x35)
	colorScheme.LineColor = gvcolor.MakeColor(th.TableSelectedBg).MulAlpha(0x80)
	colorScheme.LineNumberColor = gvcolor.MakeColor(th.SecondaryTextColor).MulAlpha(0xb6)

	styleName := "tango"
	if th.IsDark() {
		styleName = "dracula"
	}

	syntaxStyles, _ := extractStylesFromChroma(styleName)
	var bg gvcolor.Color
	for _, style := range syntaxStyles {
		colorScheme.AddStyle(style.scope, style.textStyle, style.color, style.bg)
		if style.bg.IsSet() {
			bg = style.bg
		}
	}

	if !bg.IsSet() {
		if th.IsDark() {
			bg = gvcolor.MakeColor(color.NRGBA{R: 30, G: 30, B: 30, A: 255})
		} else {
			bg = gvcolor.MakeColor(color.NRGBA{R: 250, G: 250, B: 250, A: 255})
		}
	}
	colorScheme.Background = bg

	c.editor.WithOptions(
		gvcode.WithFont(font.Font{Typeface: "Go Mono"}),
		gvcode.WithTextSize(unit.Sp(12)),
		gvcode.WithLineHeight(unit.Sp(16), 1.0),
		gvcode.WithColorScheme(colorScheme),
		gvcode.WithLineNumber(true),
		gvcode.WithLineNumberGutterGap(unit.Dp(8)),
		gvcode.ReadOnlyMode(true),
		gvcode.WithTextAlignment(text.Start),
		gvcode.WrapLine(true),
	)
}

func (c *CodeEditor) SetCode(code string) {
	c.raw = code
	if c.lang == CodeLanguageJSON {
		code = beautifyJSON(code)
	}
	c.code = code
	c.editor.SetText(code)
	c.editor.SetSyntaxTokens(c.stylingText(code)...)
}

func (c *CodeEditor) GetCode() string {
	return c.raw
}

func (c *CodeEditor) GetText() string {
	return c.editor.Text()
}

func (c *CodeEditor) SetLanguage(lang string) {
	c.lang = lang
	c.lexer = getLexer(lang)
	c.editor.SetSyntaxTokens(c.stylingText(c.editor.Text())...)
}

func (c *CodeEditor) SetReadOnly(readOnly bool) {
	c.readOnly = readOnly
	c.editor.WithOptions(gvcode.ReadOnlyMode(readOnly))
}

func (c *CodeEditor) Editor() *gvcode.Editor {
	return c.editor
}

func (c *CodeEditor) FocusTag() any {
	return c.editor
}

func (c *CodeEditor) RequestFocus() {
	c.requestFocus = true
}

func (c *CodeEditor) SetOnTextChange(f func(text string)) {
	c.onChange = f
}

func (c *CodeEditor) SetOnTab(f func(gtx layout.Context, shift bool)) {
	c.onTab = f
}

func (c *CodeEditor) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	// Handle focus request
	if c.requestFocus {
		c.requestFocus = false
		gtx.Execute(key.FocusCmd{Tag: c.editor})
	}

	if c.styledCode == "" && c.editor.Text() != "" {
		c.editor.SetSyntaxTokens(c.stylingText(c.editor.Text())...)
	}

	// Hide line numbers when content is empty
	showLineNumbers := c.editor.Text() != ""
	c.editor.WithOptions(gvcode.WithLineNumber(showLineNumbers))

	for c.copyBtn.Clicked(gtx) {
		gtx.Execute(clipboard.WriteCmd{
			Type: "text/plain",
			Data: io.NopCloser(strings.NewReader(c.editor.Text())),
		})
	}

	// Focus handling and editor updates
	for {
		ev, ok := c.editor.Update(gtx)
		if !ok {
			break
		}
		if _, ok := ev.(gvcode.ChangeEvent); ok {
			c.code = c.editor.Text()
			c.editor.SetSyntaxTokens(c.stylingText(c.code)...)
			if c.onChange != nil {
				c.onChange(c.code)
			}
		}
	}

	// Handle TAB key for focus navigation when editor is focused
	if c.onTab != nil {
		for {
			ev, ok := gtx.Event(
				key.Filter{Focus: c.editor, Name: key.NameTab, Optional: key.ModShift},
			)
			if !ok {
				break
			}
			if e, ok := ev.(key.Event); ok && e.State == key.Press {
				c.onTab(gtx, e.Modifiers.Contain(key.ModShift))
			}
		}
	}

	// Handle clicks to focus even in readonly mode
	for {
		e, ok := gtx.Event(pointer.Filter{Target: c.editor, Kinds: pointer.Press})
		if !ok {
			break
		}
		if x, ok := e.(pointer.Event); ok && x.Kind == pointer.Press {
			gtx.Execute(key.FocusCmd{Tag: c.editor})
		}
	}

	c.focused = gtx.Source.Focused(c.editor)

	c.border.Color = th.BorderColor
	c.border.Width = unit.Dp(1)

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: unit.Dp(4)}.Layout(cgtx, func(ccgtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween, Alignment: layout.Middle}.Layout(ccgtx,
					layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
						lbl := material.Label(th.Material(), unit.Sp(10), c.lang)
						lbl.Color = th.SecondaryTextColor
						return lbl.Layout(cccgtx)
					}),
					layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
						btn := material.Button(th.Material(), &c.copyBtn, "Copy")
						btn.TextSize = unit.Sp(10)
						btn.Background = th.ActionButtonBgColor
						btn.Color = th.ButtonTextColor
						btn.Inset = layout.UniformInset(unit.Dp(4))
						return btn.Layout(cccgtx)
					}),
				)
			})
		}),
		layout.Flexed(1, func(cgtx layout.Context) layout.Dimensions {
			return layout.Stack{}.Layout(cgtx,
				layout.Stacked(func(gtx1 layout.Context) layout.Dimensions {
					borderColor := th.BorderColor
					if c.focused {
						borderColor = th.BorderColorFocused
					}
					c.border.Color = borderColor

					return c.border.Layout(gtx1, func(gtx2 layout.Context) layout.Dimensions {
						return layout.Stack{}.Layout(gtx2,
							layout.Stacked(func(gtx3 layout.Context) layout.Dimensions {
								// Fix: We provide exactly the same height as the stack to prevent jiggles
								// while allowing the editor to render its full content for scrolling.
								return layout.Inset{
									Top:    unit.Dp(4),
									Left:   unit.Dp(4),
									Bottom: unit.Dp(4),
									Right:  unit.Dp(4),
								}.Layout(gtx3, func(gtx4 layout.Context) layout.Dimensions {
									// Force the editor to use the full available space
									// this helps in stabilizing the layout and scroll behavior.
									gtx4.Constraints.Min = gtx4.Constraints.Max
									editorDims := c.editor.Layout(gtx4, th.Material().Shaper)
									// Ensure the reported size matches the forced constraints
									editorDims.Size = gtx4.Constraints.Min
									return editorDims
								})
							}),
							layout.Expanded(func(gtx5 layout.Context) layout.Dimensions {
								minX, maxX, minY, maxY := c.editor.ScrollRatio()
								scrollIndicatorColor := gvcolor.MakeColor(th.Palette.Fg).MulAlpha(0x30)

								if maxY-minY < 1 {
									c.yScroll.Update(gtx5, layout.Vertical, minY, maxY)
									if d := c.yScroll.ScrollDistance(); d != 0 {
										c.editor.Scroll(gtx5, 0, d)
										gtx5.Execute(op.InvalidateCmd{})
									}
									if c.yScroll.Dragging() {
										gtx5.Execute(op.InvalidateCmd{})
									}
									layout.E.Layout(gtx5, func(gtx10 layout.Context) layout.Dimensions {
										gtx10.Constraints.Min.X = 0
										bar := material.Scrollbar(th.Material(), &c.yScroll)
										bar.Indicator.Color = scrollIndicatorColor.NRGBA()
										return bar.Layout(gtx10, layout.Vertical, minY, maxY)
									})
								}

								if maxX-minX < 1 {
									c.xScroll.Update(gtx5, layout.Horizontal, minX, maxX)
									if d := c.xScroll.ScrollDistance(); d != 0 {
										c.editor.Scroll(gtx5, d, 0)
										gtx5.Execute(op.InvalidateCmd{})
									}
									if c.xScroll.Dragging() {
										gtx5.Execute(op.InvalidateCmd{})
									}
									layout.S.Layout(gtx5, func(gtx11 layout.Context) layout.Dimensions {
										gtx11.Constraints.Min.Y = 0
										bar := material.Scrollbar(th.Material(), &c.xScroll)
										bar.Indicator.Color = scrollIndicatorColor.NRGBA()
										rInset := unit.Dp(0)
										if maxY-minY < 1 {
											rInset = unit.Dp(10)
										}
										return layout.Inset{Right: rInset}.Layout(gtx11, func(gtx12 layout.Context) layout.Dimensions {
											return bar.Layout(gtx12, layout.Horizontal, minX, maxX)
										})
									})
								}
								return layout.Dimensions{Size: gtx5.Constraints.Min}
							}),
						)
					})
				}),
				layout.Expanded(func(gtx13 layout.Context) layout.Dimensions {
					if c.focused {
						DrawFocusRing(gtx13, th.BorderColorFocused, gtx13.Constraints.Min, gtx13.Dp(unit.Dp(8)))
					}
					return layout.Dimensions{Size: gtx13.Constraints.Min}
				}),
			)
		}),
	)
}

func (c *CodeEditor) stylingText(input string) []syntax.Token {
	if c.styledCode == input {
		return c.tokens
	}

	var tokens []syntax.Token
	offset := 0
	iterator, err := c.lexer.Tokenise(nil, input)
	if err != nil {
		return tokens
	}

	for _, token := range iterator.Tokens() {
		gtoken := syntax.Token{
			Start: offset,
			End:   offset + len([]rune(token.Value)),
			Scope: syntax.StyleScope(token.Type.String()),
		}
		tokens = append(tokens, gtoken)
		offset = gtoken.End
	}

	c.styledCode = input
	c.tokens = tokens
	return tokens
}

func getLexer(lang string) chroma.Lexer {
	lexer := lexers.Get(lang)
	if lexer == nil {
		lexer = lexers.Fallback
	}
	return chroma.Coalesce(lexer)
}

func beautifyJSON(inputJSON string) string {
	if inputJSON == "" {
		return ""
	}
	// Check if input is valid JSON before attempting to beautify
	var raw interface{}
	if err := json.Unmarshal([]byte(inputJSON), &raw); err != nil {
		// Not valid JSON, return as-is
		return inputJSON
	}
	// Use simpler Pretty function first, it's very reliable
	res := pretty.Pretty([]byte(inputJSON))
	if len(res) == 0 {
		return inputJSON
	}
	return string(res)
}

func extractStylesFromChroma(styleName string) ([]colorStyle, error) {
	if st, ok := registry[styleName]; ok {
		return st, nil
	}

	chromaStyle := styles.Get(styleName)
	if chromaStyle == nil {
		return nil, fmt.Errorf("style %s not found", styleName)
	}

	var customStyles []colorStyle
	for _, tokenType := range chromaStyle.Types() {
		entry := chromaStyle.Get(tokenType)
		custom := colorStyle{
			scope:     syntax.StyleScope(tokenType.String()),
			textStyle: extractTextStyle(entry),
			color:     extractColor(entry.Colour),
			bg:        gvcolor.Color{}, // Token background should be transparent to not interfere with selection
		}
		customStyles = append(customStyles, custom)
	}

	registry[styleName] = customStyles
	return customStyles, nil
}

func extractTextStyle(entry chroma.StyleEntry) syntax.TextStyle {
	var textStyle syntax.TextStyle
	if entry.Bold == chroma.Yes {
		textStyle |= syntax.Bold
	}
	if entry.Italic == chroma.Yes {
		textStyle |= syntax.Italic
	}
	if entry.Underline == chroma.Yes {
		textStyle |= syntax.Underline
	}
	return textStyle
}

func extractColor(c chroma.Colour) gvcolor.Color {
	if !c.IsSet() {
		return gvcolor.Color{}
	}
	return gvcolor.MakeColor(color.NRGBA{
		R: c.Red(),
		G: c.Green(),
		B: c.Blue(),
		A: 0xff,
	})
}
