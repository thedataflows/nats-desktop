package components

import (
	"image"
	"image/color"
	"math"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/thedataflows/nats-desktop/internal/ui/theme"
)

type ButtonStyle struct {
	Text         string
	Icon         Icon
	IconPosition IconPosition
	// Color is the text color.
	Color        color.NRGBA
	Font         font.Font
	TextSize     unit.Sp
	Background   color.NRGBA
	CornerRadius unit.Dp
	Inset        layout.Inset
	Button       *widget.Clickable
	shaper       *text.Shaper

	IconSize  unit.Sp
	IconInset layout.Inset
}

type ButtonLayoutStyle struct {
	Background   color.NRGBA
	CornerRadius unit.Dp
	Button       *widget.Clickable
}

type IconButtonStyle struct {
	Background color.NRGBA
	// Color is the icon color.
	Color color.NRGBA
	Icon  Icon
	// Size is the icon size.
	Size        unit.Dp
	Inset       layout.Inset
	Button      *widget.Clickable
	Description string
}

func Button(th *theme.Theme, button *widget.Clickable, icon Icon, iconPosition IconPosition, txt string) ButtonStyle {
	b := ButtonStyle{
		Text:         txt,
		Icon:         icon,
		IconPosition: iconPosition,
		Color:        th.Palette.ContrastFg,
		CornerRadius: 8,
		Background:   th.Palette.ContrastBg,
		TextSize:     th.TextSize,
		Inset: layout.Inset{
			Top: 8, Bottom: 8,
			Left: 8, Right: 8,
		},
		Button:    button,
		shaper:    th.Shaper,
		IconInset: layout.Inset{Right: unit.Dp(5)},
		IconSize:  unit.Sp(18),
	}
	b.Font.Typeface = th.Face
	return b
}

func (b ButtonStyle) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	// Enforce minimum height on the context before layout
	minHeight := gtx.Dp(unit.Dp(32))
	if gtx.Constraints.Min.Y < minHeight {
		gtx.Constraints.Min.Y = minHeight
	}
	minWidth := gtx.Dp(unit.Dp(80))
	if gtx.Constraints.Min.X < minWidth {
		gtx.Constraints.Min.X = minWidth
	}

	for b.Button.Clicked(gtx) {
		gtx.Execute(key.FocusCmd{Tag: b.Button})
	}

	for {
		ev, ok := gtx.Event(
			key.Filter{Focus: b.Button, Name: key.NameEnter},
			key.Filter{Focus: b.Button, Name: key.NameReturn},
		)
		if !ok {
			break
		}

		if ke, ok := ev.(key.Event); ok && ke.State == key.Press {
			// Click on enter/space handled by caller usually
		}
	}

	dims := ButtonLayoutStyle{
		Background:   b.Background,
		CornerRadius: b.CornerRadius,
		Button:       b.Button,
	}.Layout(gtx, func(cgtx layout.Context) layout.Dimensions {
		iconDims := layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
			if b.Icon != nil {
				return b.IconInset.Layout(ccgtx, func(cccgtx layout.Context) layout.Dimensions {
					cccgtx.Constraints.Min.X = cccgtx.Dp(unit.Dp(b.IconSize))
					cccgtx.Constraints.Max.X = cccgtx.Dp(unit.Dp(b.IconSize))
					return b.Icon.Layout(cccgtx, b.Color)
				})
			}
			return layout.Dimensions{}
		})
		labelDims := layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
			lb := material.Label(th.Material(), b.TextSize, b.Text)
			lb.Font = b.Font
			lb.Color = b.Color
			return lb.Layout(ccgtx)
		})

		items := []layout.FlexChild{iconDims, labelDims}
		if b.Icon != nil && b.IconPosition == IconPositionEnd {
			items = []layout.FlexChild{labelDims, iconDims}

			b.Inset.Right = unit.Dp(5)
		}

		return b.Inset.Layout(cgtx, func(ccgtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(ccgtx,
				items...,
			)
		})
	})

	if gtx.Focused(b.Button) {
		focusCol := th.BorderColorFocused
		if b.Background == th.ActionButtonBgColor {
			focusCol = theme.White
		}
		DrawFocusRing(gtx, focusCol, dims.Size, gtx.Dp(unit.Dp(8)))
	}

	return dims
}

func (b ButtonLayoutStyle) Layout(gtx layout.Context, w layout.Widget) layout.Dimensions {
	// Enforce minimum dimensions for buttons to prevent squashing
	minHeight := gtx.Dp(unit.Dp(32))
	if gtx.Constraints.Min.Y < minHeight {
		gtx.Constraints.Min.Y = minHeight
	}
	minWidth := gtx.Dp(unit.Dp(80))
	if gtx.Constraints.Min.X < minWidth {
		gtx.Constraints.Min.X = minWidth
	}
	return b.Button.Layout(gtx, func(cgtx layout.Context) layout.Dimensions {
		semantic.Button.Add(cgtx.Ops)
		return layout.Background{}.Layout(cgtx,
			func(ccgtx layout.Context) layout.Dimensions {
				rr := ccgtx.Dp(b.CornerRadius)
				defer clip.UniformRRect(image.Rectangle{Max: ccgtx.Constraints.Min}, rr).Push(ccgtx.Ops).Pop()
				background := b.Background
				switch {
				case !ccgtx.Enabled():
					background = Disabled(b.Background)
				case b.Button.Hovered():
					background = Hovered(b.Background)
				}
				paint.Fill(ccgtx.Ops, background)
				for _, c := range b.Button.History() {
					drawInk(ccgtx, c)
				}
				return layout.Dimensions{Size: ccgtx.Constraints.Min}
			},
			func(ccgtx layout.Context) layout.Dimensions {
				return layout.Center.Layout(ccgtx, w)
			},
		)
	})
}

func drawInk(gtx layout.Context, c widget.Press) {
	// duration is the number of seconds for the
	// completed animation: expand while fading in, then
	// out.
	const (
		expandDuration = float32(0.5)
		fadeDuration   = float32(0.9)
	)

	now := gtx.Now

	t := float32(now.Sub(c.Start).Seconds())

	end := c.End
	if end.IsZero() {
		// If the press hasn't ended, don't fade-out.
		end = now
	}

	endt := float32(end.Sub(c.Start).Seconds())

	// Compute the fade-in/out position in [0;1].
	var alphat float32
	{
		var haste float32
		if c.Cancelled {
			// If the press was cancelled before the inkwell
			// was fully faded in, fast forward the animation
			// to match the fade-out.
			if h := 0.5 - endt/fadeDuration; h > 0 {
				haste = h
			}
		}
		// Fade in.
		half1 := t/fadeDuration + haste
		if half1 > 0.5 {
			half1 = 0.5
		}

		// Fade out.
		half2 := float32(now.Sub(end).Seconds())
		half2 /= fadeDuration
		half2 += haste
		if half2 > 0.5 {
			// Too old.
			return
		}

		alphat = half1 + half2
	}

	// Compute the expand position in [0;1].
	sizet := t
	if c.Cancelled {
		// Freeze expansion of cancelled presses.
		sizet = endt
	}
	sizet /= expandDuration

	// Animate only ended presses, and presses that are fading in.
	if !c.End.IsZero() || sizet <= 1.0 {
		gtx.Execute(op.InvalidateCmd{})
	}

	if sizet > 1.0 {
		sizet = 1.0
	}

	if alphat > .5 {
		// Start fadeout after half the animation.
		alphat = 1.0 - alphat
	}
	// Twice the speed to attain fully faded in at 0.5.
	t2 := alphat * 2
	// Beziér ease-in curve.
	alphaBezier := t2 * t2 * (3.0 - 2.0*t2)
	sizeBezier := sizet * sizet * (3.0 - 2.0*sizet)
	size := gtx.Constraints.Min.X
	if h := gtx.Constraints.Min.Y; h > size {
		size = h
	}
	// Cover the entire constraints min rectangle and
	// apply curve values to size and color.
	size = int(float32(size) * 2 * float32(math.Sqrt(2)) * sizeBezier)
	alpha := 0.7 * alphaBezier
	const col = 0.8
	ba, bc := byte(alpha*0xff), byte(col*0xff)
	rgba := MulAlpha(color.NRGBA{A: 0xff, R: bc, G: bc, B: bc}, ba)
	ink := paint.ColorOp{Color: rgba}
	ink.Add(gtx.Ops)
	rr := size / 2
	defer op.Offset(c.Position.Add(image.Point{
		X: -rr,
		Y: -rr,
	})).Push(gtx.Ops).Pop()
	defer clip.UniformRRect(image.Rectangle{Max: image.Pt(size, size)}, rr).Push(gtx.Ops).Pop()
	paint.PaintOp{}.Add(gtx.Ops)
}

func SecondaryButton(th *theme.Theme, button *widget.Clickable, icon Icon, iconPosition IconPosition, txt string) ButtonStyle {
	b := Button(th, button, icon, iconPosition, txt)
	b.Background = th.Palette.ContrastBg
	b.Color = th.Palette.ContrastFg
	return b
}
