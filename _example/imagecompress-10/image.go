package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"

	"github.com/pkg/errors"
)

type macroblock uint8

const (
	mb8_4x8_4  macroblock = 1
	mb16_2x8_4 macroblock = 2
	mb16_2x2   macroblock = 3
	mb32x32    macroblock = 4
	mb16x16    macroblock = 5
	mb8x8x4    macroblock = 6
)

type macroblockPartition struct {
	X, Y, Size int
}

var (
	mbPartitoins = map[macroblock][]macroblockPartition{
		mb8_4x8_4: {
			{0, 0, 8}, {8, 0, 8}, {16, 0, 8}, {24, 0, 8},
			{0, 8, 8}, {8, 8, 8}, {16, 8, 8}, {24, 8, 8},
			{0, 16, 8}, {8, 16, 8}, {16, 16, 8}, {24, 16, 8},
			{0, 24, 8}, {8, 24, 8}, {16, 24, 8}, {24, 24, 8},
		},
		mb16_2x8_4: {
			{0, 0, 16},
			{16, 0, 8}, {24, 0, 8}, {16, 8, 8}, {24, 8, 8},
			{0, 16, 8}, {8, 16, 8}, {0, 24, 8}, {8, 24, 8},
			{16, 16, 16},
		},
		mb16_2x2: {
			{0, 0, 16},
			{16, 0, 16},
			{0, 16, 16},
			{16, 16, 16},
		},
		mb16x16: {
			{0, 0, 16},
		},
		mb32x32: {
			{0, 0, 32},
		},
		mb8x8x4: {
			{0, 0, 8}, {8, 0, 8},
			{0, 8, 8}, {8, 8, 8},
		},
	}
)

func qtBySize(size int) [3]QuantizationTable {
	switch macroblock(size) {
	case 8:
		return [3]QuantizationTable{dcQT8, lumaQT8, chromaQT8}
	case 16:
		return [3]QuantizationTable{dcQT16, lumaQT16, chromaQT16}
	case 32:
		return [3]QuantizationTable{dcQT32, lumaQT32, chromaQT32}
	}
	panic(fmt.Sprintf("size:%d not found", size))
}

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

func clampU8(v int16) uint8 {
	if v < 0 {
		return 0
	}
	if 255 < v {
		return 255
	}
	return uint8(v)
}

type ImageReader struct {
	img           *image.YCbCr
	width, height int
}

func (r *ImageReader) headtailY(w, h int, n int) (uint8, uint8) {
	i := (h * r.img.YStride) + w
	if len(r.img.Y) < i {
		return 0, 0
	}
	if len(r.img.Y) < i+n {
		return r.img.Y[i], 0
	}
	return r.img.Y[i], r.img.Y[i+n]
}

func (r *ImageReader) headtailCb(w, h int, n int) (uint8, uint8) {
	i1 := r.img.COffset(w*2, h*2)
	i2 := r.img.COffset((w+n)*2, h*2)
	if len(r.img.Cb) <= i1 {
		return 0, 0
	}
	if len(r.img.Cb) <= i2 {
		return r.img.Cb[i1], 0
	}
	return r.img.Cb[i1], r.img.Cb[i2]
}

func (r *ImageReader) headtailCr(w, h int, n int) (uint8, uint8) {
	i1 := r.img.COffset(w*2, h*2)
	i2 := r.img.COffset((w+n)*2, h*2)
	if len(r.img.Cr) <= i1 {
		return 0, 0
	}
	if len(r.img.Cr) <= i2 {
		return r.img.Cr[i1], 0
	}
	return r.img.Cr[i1], r.img.Cr[i2]
}

func (r *ImageReader) CalcMacroblockY(w, h int) macroblock {
	if similarblock(w, h, 16, r.headtailY) {
		if similarblockNeighbor(w, h, 16, 8, r.headtailY) {
			return mb16_2x2
		}
		return mb16_2x8_4
	}
	return mb8_4x8_4
}

func (r *ImageReader) CalcMacroblockCb(w, h int) macroblock {
	if similarblock(w, h, 16, r.headtailCb) {
		return mb16x16
	}
	return mb8x8x4
}

func (r *ImageReader) CalcMacroblockCr(w, h int) macroblock {
	if similarblock(w, h, 16, r.headtailCr) {
		return mb16x16
	}
	return mb8x8x4
}

func (r *ImageReader) RowY(x, y int, size int, prediction int16) []int16 {
	plane := make([]int16, size)
	for i := 0; i < size; i += 1 {
		px, py := boundaryRepeat(r.width, r.height, x+i, y)

		plane[i] = int16(r.img.Y[r.img.YOffset(px, py)]) - prediction
	}
	return plane
}

func (r *ImageReader) RowCb(x, y int, size int, prediction int16) []int16 {
	plane := make([]int16, size)
	for i := 0; i < size; i += 1 {
		px, py := boundaryRepeat(r.width, r.height, (x+i)*2, y*2)

		plane[i] = int16(r.img.Cb[r.img.COffset(px, py)]) - prediction
	}
	return plane
}

func (r *ImageReader) RowCr(x, y int, size int, prediction int16) []int16 {
	plane := make([]int16, size)
	for i := 0; i < size; i += 1 {
		px, py := boundaryRepeat(r.width, r.height, (x+i)*2, y*2)

		plane[i] = int16(r.img.Cr[r.img.COffset(px, py)]) - prediction
	}
	return plane
}

func newImageReader(img *image.YCbCr) *ImageReader {
	return &ImageReader{
		img:    img,
		width:  img.Rect.Dx(),
		height: img.Rect.Dy(),
	}
}

type ImagePredictor struct {
	img           *image.YCbCr
	width, height int
}

func (p *ImagePredictor) UpdateY(x, y int, size int, plane []int16, prediction int16) {
	for i := 0; i < size; i += 1 {
		if p.width <= x+i || p.height <= y {
			continue
		}
		p.img.Y[p.img.YOffset(x+i, y)] = clampU8(plane[i] + prediction)
	}
}

func (p *ImagePredictor) UpdateCb(x, y int, size int, plane []int16, prediction int16) {
	for i := 0; i < size; i += 1 {
		px := (x + i) * 2
		py := y * 2
		if p.width <= px || p.height <= py {
			continue
		}
		p.img.Cb[p.img.COffset(px, py)] = clampU8(plane[i] + prediction)
	}
}

func (p *ImagePredictor) UpdateCr(x, y int, size int, plane []int16, prediction int16) {
	for i := 0; i < size; i += 1 {
		px := (x + i) * 2
		py := y * 2
		if p.width <= px || p.height <= py {
			continue
		}
		p.img.Cr[p.img.COffset(px, py)] = clampU8(plane[i] + prediction)
	}
}

func (p *ImagePredictor) PredictY(x, y int, size int) int16 {
	return p.predictDC(p.img.Y, p.img.YStride, p.img.YOffset(x, y), x, y, size)
}

func (p *ImagePredictor) PredictCb(x, y int, size int) int16 {
	return p.predictDC(p.img.Cb, p.img.CStride, p.img.COffset(x*2, y*2), x, y, size)
}

func (p *ImagePredictor) PredictCr(x, y int, size int) int16 {
	return p.predictDC(p.img.Cr, p.img.CStride, p.img.COffset(x*2, y*2), x, y, size)
}

func (p *ImagePredictor) predictDC(data []byte, stride int, offset, x, y int, size int) int16 {
	sum := 0
	count := 0

	// Top neighbor
	if 0 < y {
		topStart := offset - stride
		for i := 0; i < size; i += 1 {
			if topStart+i < len(data) {
				sum += int(data[topStart+i])
				count++
			}
		}
	}

	// Left neighbor
	if 0 < x {
		leftStart := offset - 1
		for i := 0; i < size; i += 1 {
			if leftStart+(i*stride) < len(data) {
				sum += int(data[leftStart+(i*stride)])
				count++
			}
		}
	}

	if count == 0 {
		return 128
	}

	// average
	return int16(sum / count)
}

func (p *ImagePredictor) Deblocking() {
	for h := 0; h < p.height; h += 1 {
		for w := 15; w < p.width-1; w += 15 {
			p0 := int16(p.img.Y[p.img.YOffset(w-2, h)]) // x=13
			p1 := int16(p.img.Y[p.img.YOffset(w-1, h)]) // x=14
			p2 := int16(p.img.Y[p.img.YOffset(w, h)])   // x=15
			p3 := int16(p.img.Y[p.img.YOffset(w+1, h)]) // x=16
			p4 := int16(p.img.Y[p.img.YOffset(w+2, h)]) // x=17
			avg := (p0 + p1 + p2 + p3 + p4) / 5
			p.img.Y[p.img.YOffset(w, h)] = uint8((p2*2 + avg) / 3)
			p.img.Y[p.img.YOffset(w+1, h)] = uint8((p3*2 + avg) / 3)
		}
	}
}

func newImagePredictor(rect image.Rectangle) *ImagePredictor {
	return &ImagePredictor{
		img:    image.NewYCbCr(rect, image.YCbCrSubsampleRatio420),
		width:  rect.Dx(),
		height: rect.Dy(),
	}
}

type headtailFunc func(w, h int, n int) (uint8, uint8)

func similarblock(w, h int, size int, headtail headtailFunc) bool {
	h0, t0 := headtail(w, h, size)
	h1, t1 := headtail(w, h+(size-1), size)

	mi := min(min(h0, h1), min(t0, t1))
	ma := max(max(h0, h1), max(t0, t1))
	if (ma - mi) < 48 { // 48/255 = 0.1882
		return true
	}
	return false
}

func similarblockNeighbor(w, h int, size, next int, headtail headtailFunc) bool {
	h0, t0 := headtail(w, h, size)
	h1, t1 := headtail(w, h+(size-1), size)
	h2, t2 := headtail(w, h+(size+next-1), size)

	mi := min(min(min(h0, h1), min(t0, t1)), min(h2, t2))
	ma := max(max(max(h0, h1), max(t0, t1)), max(h2, t2))
	if (ma - mi) < 24 { // 24/255 = 0.0941
		return true
	}
	return false
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
