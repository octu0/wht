package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"

	"github.com/pkg/errors"
)

func boundaryRepeat(width, height uint16, px, py uint16) (uint16, uint16) {
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
	width, height uint16
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

func (r *ImageReader) RowY(x, y uint16, size uint16, prediction int16) []int16 {
	plane := make([]int16, size)
	for i := uint16(0); i < size; i += 1 {
		px, py := boundaryRepeat(r.width, r.height, x+i, y)
		plane[i] = int16(r.img.Y[r.img.YOffset(int(px), int(py))]) - prediction
	}
	return plane
}

func (r *ImageReader) RowCb(x, y uint16, size uint16, prediction int16) []int16 {
	plane := make([]int16, size)
	for i := uint16(0); i < size; i += 1 {
		px, py := boundaryRepeat(r.width, r.height, (x+i)*2, y*2)
		plane[i] = int16(r.img.Cb[r.img.COffset(int(px), int(py))]) - prediction
	}
	return plane
}

func (r *ImageReader) RowCr(x, y uint16, size uint16, prediction int16) []int16 {
	plane := make([]int16, size)
	for i := uint16(0); i < size; i += 1 {
		px, py := boundaryRepeat(r.width, r.height, (x+i)*2, y*2)
		plane[i] = int16(r.img.Cr[r.img.COffset(int(px), int(py))]) - prediction
	}
	return plane
}

func newImageReader(img *image.YCbCr) *ImageReader {
	return &ImageReader{
		img:    img,
		width:  uint16(img.Rect.Dx()),
		height: uint16(img.Rect.Dy()),
	}
}

type ImagePredictor struct {
	img           *image.YCbCr
	width, height uint16
}

func (p *ImagePredictor) UpdateY(x, y uint16, size uint16, plane []int16, prediction int16) {
	for i := uint16(0); i < size; i += 1 {
		if p.width <= x+i || p.height <= y {
			continue
		}
		p.img.Y[p.img.YOffset(int(x+i), int(y))] = clampU8(plane[i] + prediction)
	}
}

func (p *ImagePredictor) UpdateCb(x, y uint16, size uint16, plane []int16, prediction int16) {
	for i := uint16(0); i < size; i += 1 {
		px, py := (x+i)*2, y*2
		if p.width <= px || p.height <= py {
			continue
		}
		p.img.Cb[p.img.COffset(int(px), int(py))] = clampU8(plane[i] + prediction)
	}
}

func (p *ImagePredictor) UpdateCr(x, y uint16, size uint16, plane []int16, prediction int16) {
	for i := uint16(0); i < size; i += 1 {
		px, py := (x+i)*2, y*2
		if p.width <= px || p.height <= py {
			continue
		}
		p.img.Cr[p.img.COffset(int(px), int(py))] = clampU8(plane[i] + prediction)
	}
}

func (p *ImagePredictor) PredictY(x, y uint16, size uint16) int16 {
	return p.predictDC(p.img.Y, p.img.YStride, p.img.YOffset(int(x), int(y)), x, y, size)
}

func (p *ImagePredictor) PredictCb(x, y uint16, size uint16) int16 {
	return p.predictDC(p.img.Cb, p.img.CStride, p.img.COffset(int(x*2), int(y*2)), x, y, size)
}

func (p *ImagePredictor) PredictCr(x, y uint16, size uint16) int16 {
	return p.predictDC(p.img.Cr, p.img.CStride, p.img.COffset(int(x*2), int(y*2)), x, y, size)
}

func (p *ImagePredictor) predictDC(data []byte, stride int, offset int, x, y uint16, size uint16) int16 {
	sum, count := 0, 0
	if 0 < y {
		topStart := offset - stride
		for i := 0; i < int(size); i += 1 {
			if topStart+i < len(data) {
				sum += int(data[topStart+i])
				count += 1
			}
		}
	}
	if 0 < x {
		leftStart := offset - 1
		for i := 0; i < int(size); i += 1 {
			if leftStart+(i*stride) < len(data) {
				sum += int(data[leftStart+(i*stride)])
				count += 1
			}
		}
	}

	if count == 0 {
		return 128
	}
	// average
	return int16(sum / count)
}

func newImagePredictor(rect image.Rectangle) *ImagePredictor {
	return &ImagePredictor{
		img:    image.NewYCbCr(rect, image.YCbCrSubsampleRatio420),
		width:  uint16(rect.Dx()),
		height: uint16(rect.Dy()),
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
