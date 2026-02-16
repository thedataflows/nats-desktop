package assets

import (
	"bytes"
	"embed"
	"fmt"
	"image"
	_ "image/png"

	"gioui.org/op/paint"
)

//go:embed icons/*
var icons embed.FS

func LoadIcon(size string) (image.Image, error) {
	fileName := fmt.Sprintf("nats-plain-%s.png", size)
	file, err := icons.ReadFile(fmt.Sprintf("icons/%s", fileName))
	if err != nil {
		return nil, err
	}

	img, _, err := image.Decode(bytes.NewReader(file))
	if err != nil {
		return nil, err
	}

	return img, nil
}

func MustLoadIcon(size string) image.Image {
	img, err := LoadIcon(size)
	if err != nil {
		panic(fmt.Sprintf("failed to load icon %s: %v", size, err))
	}
	return img
}

func MustLoadIconOp(size string) paint.ImageOp {
	img := MustLoadIcon(size)
	return paint.NewImageOp(img)
}

var (
	Icon32  = MustLoadIconOp("32px")
	Icon64  = MustLoadIconOp("64px")
	Icon128 = MustLoadIconOp("128px")
	Icon256 = MustLoadIconOp("256px")
	Icon512 = MustLoadIconOp("512px")

	// AppIcon is a smaller one as requested
	AppIcon = MustLoadIcon("32px")
)
