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

func (r *ImageReader) Width() uint16 {
	return r.width
}

func (r *ImageReader) Height() uint16 {
	return r.height
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

type Image16 struct {
	Y, Cb, Cr     [][]int16
	Width, Height uint16
}

func (i *Image16) GetY(x, y uint16, size uint16) [][]int16 {
	plane := make([][]int16, size)
	for h := uint16(0); h < size; h += 1 {
		plane[h] = make([]int16, size)
		for w := uint16(0); w < size; w += 1 {
			px, py := boundaryRepeat(i.Width, i.Height, x+w, y+h)
			plane[h][w] = i.Y[py][px]
		}
	}
	return plane
}

func (i *Image16) GetCb(x, y uint16, size uint16) [][]int16 {
	plane := make([][]int16, size)
	for h := uint16(0); h < size; h += 1 {
		plane[h] = make([]int16, size)
		for w := uint16(0); w < size; w += 1 {
			px, py := boundaryRepeat(i.Width/2, i.Height/2, x+w, y+h)
			plane[h][w] = i.Cb[py][px]
		}
	}
	return plane
}

func (i *Image16) GetCr(x, y uint16, size uint16) [][]int16 {
	plane := make([][]int16, size)
	for h := uint16(0); h < size; h += 1 {
		plane[h] = make([]int16, size)
		for w := uint16(0); w < size; w += 1 {
			px, py := boundaryRepeat(i.Width/2, i.Height/2, x+w, y+h)
			plane[h][w] = i.Cr[py][px]
		}
	}
	return plane
}

func (i *Image16) UpdateY(data [][]int16, prediction int16, startX, startY uint16, size uint16) {
	for h := uint16(0); h < size; h += 1 {
		if i.Height <= startY+h {
			continue
		}
		for w := uint16(0); w < size; w += 1 {
			i.Y[startY+h][startX+w] = data[h][w] + prediction
		}
	}
}

func (i *Image16) UpdateCb(data [][]int16, prediction int16, startX, startY uint16, size uint16) {
	for h := uint16(0); h < size; h += 1 {
		if i.Height/2 <= startY+h {
			continue
		}
		for w := uint16(0); w < size; w += 1 {
			i.Cb[startY+h][startX+w] = data[h][w] + prediction
		}
	}
}

func (i *Image16) UpdateCr(data [][]int16, prediction int16, startX, startY uint16, size uint16) {
	for h := uint16(0); h < size; h += 1 {
		if i.Height/2 <= startY+h {
			continue
		}
		for w := uint16(0); w < size; w += 1 {
			i.Cr[startY+h][startX+w] = data[h][w] + prediction
		}
	}
}

func (i *Image16) ToYCbCr() *image.YCbCr {
	rect := image.Rect(0, 0, int(i.Width), int(i.Height))
	img := image.NewYCbCr(rect, image.YCbCrSubsampleRatio420)
	for y := uint16(0); y < i.Height; y += 1 {
		for x := uint16(0); x < i.Width; x += 1 {
			img.Y[img.YOffset(int(x), int(y))] = clampU8(i.Y[y][x])
		}
	}
	for y := uint16(0); y < i.Height/2; y += 1 {
		for x := uint16(0); x < i.Width/2; x += 1 {
			off := img.COffset(int(x*2), int(y*2))
			img.Cb[off] = clampU8(i.Cb[y][x])
			img.Cr[off] = clampU8(i.Cr[y][x])
		}
	}
	return img
}

func NewImage16(width, height uint16) *Image16 {
	y := make([][]int16, height)
	for i := uint16(0); i < height; i += 1 {
		y[i] = make([]int16, width) // zero clear
	}
	cb := make([][]int16, height/2)
	for i := uint16(0); i < height/2; i += 1 {
		cb[i] = make([]int16, width/2) // zero clear
	}
	cr := make([][]int16, height/2)
	for i := uint16(0); i < height/2; i += 1 {
		cr[i] = make([]int16, width/2) // zero clear
	}
	return &Image16{y, cb, cr, width, height}
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
