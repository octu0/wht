package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"

	"github.com/pkg/errors"
)

// boundaryRepeat は画像の境界処理（ミラーリング）を行います
func boundaryRepeat(width, height int, px, py int) (int, int) {
	switch {
	case width <= px:
		px = width - 1 - (px - width) // Reflection
		if px < 0 {
			px = 0 // Clamp just in case
		}
	case px < 0:
		px = -px
		if width <= px {
			px = width - 1
		}
	}
	switch {
	case height <= py:
		py = height - 1 - (py - height)
		if py < 0 {
			py = 0
		}
	case py < 0:
		py = -py
		if height <= py {
			py = height - 1
		}
	}
	return px, py
}

// clampU8 はint16をuint8の範囲にクランプします
func clampU8(v int16) uint8 {
	if v < 0 {
		return 0
	}
	if 255 < v {
		return 255
	}
	return uint8(v)
}

func pngToYCbCr(data []byte) (*image.YCbCr, error) {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, errors.WithStack(err)
	}

	ycbcr := image.NewYCbCr(img.Bounds(), image.YCbCrSubsampleRatio444)
	if err := convertYCbCr(ycbcr, img); err != nil {
		return nil, errors.WithStack(err)
	}
	return ycbcr, nil
}

func saveImage(img image.Image, name string) error {
	out, err := os.Create(name)
	if err != nil {
		return errors.WithStack(err)
	}
	defer out.Close()

	if err := png.Encode(out, img); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func convertYCbCr(dst *image.YCbCr, src image.Image) error {
	rect := src.Bounds()
	width, height := rect.Dx(), rect.Dy()

	for w := 0; w < width; w += 1 {
		for h := 0; h < height; h += 1 {
			c := src.At(w, h)
			r, g, b, _ := c.RGBA()
			y, u, v := color.RGBToYCbCr(uint8(r>>8), uint8(g>>8), uint8(b>>8))
			dst.Y[dst.YOffset(w, h)] = y
			dst.Cb[dst.COffset(w, h)] = u
			dst.Cr[dst.COffset(w, h)] = v
		}
	}
	return nil
}

// Image16 は16bitのYUVデータを保持する内部構造体です
type Image16 struct {
	Y, Cb, Cr     []int16
	Width, Height int
}

func NewImage16(w, h int) *Image16 {
	return &Image16{
		Y:      make([]int16, w*h),
		Cb:     make([]int16, (w/2)*(h/2)),
		Cr:     make([]int16, (w/2)*(h/2)),
		Width:  w,
		Height: h,
	}
}

func (i *Image16) YOffset(x, y int) int {
	return y*i.Width + x
}

func (i *Image16) COffset(x, y int) int {
	return y*(i.Width/2) + x
}

// ToYCbCr はImage16をimage.YCbCrへ変換します（クランプ処理含む）
func (i *Image16) ToYCbCr() *image.YCbCr {
	rect := image.Rect(0, 0, i.Width, i.Height)
	dst := image.NewYCbCr(rect, image.YCbCrSubsampleRatio420)
	for y := 0; y < i.Height; y += 1 {
		for x := 0; x < i.Width; x += 1 {
			dst.Y[dst.YOffset(x, y)] = clampU8(i.Y[i.YOffset(x, y)])
		}
	}
	cw, ch := i.Width/2, i.Height/2
	for y := 0; y < ch; y += 1 {
		for x := 0; x < cw; x += 1 {
			// dst.COffset expects image coordinates (so x*2, y*2 for 420)
			dst.Cb[dst.COffset(x*2, y*2)] = clampU8(i.Cb[i.COffset(x, y)])
			dst.Cr[dst.COffset(x*2, y*2)] = clampU8(i.Cr[i.COffset(x, y)])
		}
	}
	return dst
}

// YCbCrToImage16 はimage.YCbCrをImage16へ変換します
// Subsample 444 -> 420 (pick 2x, 2y)
func YCbCrToImage16(src *image.YCbCr) *Image16 {
	w, h := src.Rect.Dx(), src.Rect.Dy()
	dst := NewImage16(w, h)

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.Y[dst.YOffset(x, y)] = int16(src.Y[src.YOffset(x, y)])
		}
	}

	cw, ch := w/2, h/2
	for y := 0; y < ch; y++ {
		for x := 0; x < cw; x++ {
			// Subsample 444 -> 420 by picking pixel at 2x, 2y
			dst.Cb[dst.COffset(x, y)] = int16(src.Cb[src.COffset(x*2, y*2)])
			dst.Cr[dst.COffset(x, y)] = int16(src.Cr[src.COffset(x*2, y*2)])
		}
	}
	return dst
}