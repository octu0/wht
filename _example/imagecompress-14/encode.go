package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"io"
	"unsafe"

	"github.com/pkg/errors"
)

const (
	EncBlockSize = 16
)

func encodeBlockData(out io.Writer, data [][]int16, size, scale int) error {
	if err := binary.Write(out, binary.BigEndian, uint8(scale)); err != nil {
		return errors.WithStack(err)
	}

	quantizeBlock(data, size, scale)

	zigzag := Zigzag(data, size)

	// int16 -> uint16
	u16data := unsafe.Slice((*uint16)(unsafe.Pointer(&zigzag[0])), len(zigzag))

	rw := NewRiceWriter[uint16](NewBitWriter(out))
	if err := blockRLEEncode(rw, u16data); err != nil {
		return errors.WithStack(err)
	}
	if err := rw.Flush(); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func encodeSubband(plane []int16, width, height int, startX, startY, endX, endY int, scaler *RateController) ([]*bytes.Buffer, error) {
	out := make([]*bytes.Buffer, 0)
	blockSize := EncBlockSize // Fixed block size for encoding
	scaleVal := 1

	for y := startY; y < endY; y += blockSize {
		for x := startX; x < endX; x += blockSize {
			// Extract block
			block := make([][]int16, blockSize)
			for i := 0; i < blockSize; i += 1 {
				block[i] = make([]int16, blockSize)
				for j := 0; j < blockSize; j += 1 {
					px, py := x+j, y+i
					val := int16(0)
					if px < width && py < height {
						val = plane[(py*width)+px]
					}
					block[i][j] = val
				}
			}

			// Encode block (Quantize + RLE)
			buf := bytes.NewBuffer(make([]byte, 0, blockSize*blockSize))
			if err := encodeBlockData(buf, block, blockSize, scaleVal); err != nil {
				return nil, errors.WithStack(err)
			}
			out = append(out, buf)
			scaleVal = scaler.CalcScale(buf.Len()*8, blockSize*blockSize)
		}
	}
	return out, nil
}

// encodeLayer は1つの解像度レイヤーをエンコードし、次のレイヤーへの入力(LL成分)を返します
func encodeLayer(src *Image16, maxbitrate int) ([]byte, *Image16, error) {
	bufY := make([]*bytes.Buffer, 0)
	bufCb := make([]*bytes.Buffer, 0)
	bufCr := make([]*bytes.Buffer, 0)

	dx, dy := src.Width, src.Height

	// Y Plane DWT
	DWTPlane(src.Y, dx, dy)

	// Create nextImg (LL band)
	nextW, nextH := dx/2, dy/2
	nextImg := NewImage16(nextW, nextH)

	// Copy LL to nextImg
	for y := 0; y < nextH; y += 1 {
		for x := 0; x < nextW; x += 1 {
			nextImg.Y[nextImg.YOffset(x, y)] = src.Y[(y*dx)+x]
		}
	}

	scalerY := newRateController(maxbitrate, dx, dy)

	// Encode HL (Right-Top)
	if bufs, err := encodeSubband(src.Y, dx, dy, nextW, 0, dx, nextH, scalerY); err != nil {
		return nil, nil, errors.WithStack(err)
	} else {
		bufY = append(bufY, bufs...)
	}
	// Encode LH (Left-Bottom)
	if bufs, err := encodeSubband(src.Y, dx, dy, 0, nextH, nextW, dy, scalerY); err != nil {
		return nil, nil, errors.WithStack(err)
	} else {
		bufY = append(bufY, bufs...)
	}
	// Encode HH (Right-Bottom)
	if bufs, err := encodeSubband(src.Y, dx, dy, nextW, nextH, dx, dy, scalerY); err != nil {
		return nil, nil, errors.WithStack(err)
	} else {
		bufY = append(bufY, bufs...)
	}

	// Chroma DWT
	cw, ch := dx/2, dy/2
	DWTPlane(src.Cb, cw, ch)
	DWTPlane(src.Cr, cw, ch)

	// Copy Chroma LL to nextImg
	nextCW, nextCH := cw/2, ch/2
	for y := 0; y < nextCH; y += 1 {
		for x := 0; x < nextCW; x += 1 {
			nextImg.Cb[nextImg.COffset(x, y)] = src.Cb[(y*cw)+x]
			nextImg.Cr[nextImg.COffset(x, y)] = src.Cr[(y*cw)+x]
		}
	}

	// Encode Chroma Subbands
	// Cb
	if bufs, err := encodeSubband(src.Cb, cw, ch, nextCW, 0, cw, nextCH, scalerY); err != nil {
		return nil, nil, errors.WithStack(err)
	} else {
		bufCb = append(bufCb, bufs...)
	}
	if bufs, err := encodeSubband(src.Cb, cw, ch, 0, nextCH, nextCW, ch, scalerY); err != nil {
		return nil, nil, errors.WithStack(err)
	} else {
		bufCb = append(bufCb, bufs...)
	}
	if bufs, err := encodeSubband(src.Cb, cw, ch, nextCW, nextCH, cw, ch, scalerY); err != nil {
		return nil, nil, errors.WithStack(err)
	} else {
		bufCb = append(bufCb, bufs...)
	}

	// Cr
	if bufs, err := encodeSubband(src.Cr, cw, ch, nextCW, 0, cw, nextCH, scalerY); err != nil {
		return nil, nil, errors.WithStack(err)
	} else {
		bufCr = append(bufCr, bufs...)
	}
	if bufs, err := encodeSubband(src.Cr, cw, ch, 0, nextCH, nextCW, ch, scalerY); err != nil {
		return nil, nil, errors.WithStack(err)
	} else {
		bufCr = append(bufCr, bufs...)
	}
	if bufs, err := encodeSubband(src.Cr, cw, ch, nextCW, nextCH, cw, ch, scalerY); err != nil {
		return nil, nil, errors.WithStack(err)
	} else {
		bufCr = append(bufCr, bufs...)
	}

	// Serialize
	out, err := serializeStreams(dx, dy, bufY, bufCb, bufCr)
	if err != nil {
		return nil, nil, err
	}
	return out, nextImg, nil
}

// encodeBase はベースレイヤー(Layer0)をエンコードします
func encodeBase(img *image.YCbCr, maxbitrate int) ([]byte, error) {
	bufY := make([]*bytes.Buffer, 0)
	bufCb := make([]*bytes.Buffer, 0)
	bufCr := make([]*bytes.Buffer, 0)

	dx, dy := img.Bounds().Dx(), img.Bounds().Dy()
	img16 := YCbCrToImage16(img)

	// Y Plane DWT
	DWTPlane(img16.Y, dx, dy)
	nextW, nextH := dx/2, dy/2

	scalerY := newRateController(maxbitrate, dx, dy)

	// Encode all subbands for Base Layer
	subbands := []struct{ sx, sy, ex, ey int }{
		{0, 0, nextW, nextH},         // LL
		{nextW, 0, dx, nextH},        // HL
		{0, nextH, nextW, dy},        // LH
		{nextW, nextH, dx, dy},       // HH
	}
	for _, s := range subbands {
		if bufs, err := encodeSubband(img16.Y, dx, dy, s.sx, s.sy, s.ex, s.ey, scalerY); err != nil {
			return nil, errors.WithStack(err)
		} else {
			bufY = append(bufY, bufs...)
		}
	}

	// Chroma DWT
	cw, ch := dx/2, dy/2
	DWTPlane(img16.Cb, cw, ch)
	DWTPlane(img16.Cr, cw, ch)
	nextCW, nextCH := cw/2, ch/2

	// Cb subbands
	cSubbands := []struct{ sx, sy, ex, ey int }{
		{0, 0, nextCW, nextCH},
		{nextCW, 0, cw, nextCH},
		{0, nextCH, nextCW, ch},
		{nextCW, nextCH, cw, ch},
	}
	for _, s := range cSubbands {
		if bufs, err := encodeSubband(img16.Cb, cw, ch, s.sx, s.sy, s.ex, s.ey, scalerY); err != nil {
			return nil, errors.WithStack(err)
		} else {
			bufCb = append(bufCb, bufs...)
		}
	}
	// Cr subbands
	for _, s := range cSubbands {
		if bufs, err := encodeSubband(img16.Cr, cw, ch, s.sx, s.sy, s.ex, s.ey, scalerY); err != nil {
			return nil, errors.WithStack(err)
		} else {
			bufCr = append(bufCr, bufs...)
		}
	}

	return serializeStreams(dx, dy, bufY, bufCb, bufCr)
}

func serializeStreams(dx, dy int, bufY, bufCb, bufCr []*bytes.Buffer) ([]byte, error) {
	ySize := 0
	for _, b := range bufY {
		ySize += b.Len()
	}
	cbSize := 0
	for _, b := range bufCb {
		cbSize += b.Len()
	}
	crSize := 0
	for _, b := range bufCr {
		crSize += b.Len()
	}

	out := bytes.NewBuffer(make([]byte, 0, 2+2+ySize+cbSize+crSize))

	if err := binary.Write(out, binary.BigEndian, uint16(dx)); err != nil {
		return nil, errors.WithStack(err)
	}
	if err := binary.Write(out, binary.BigEndian, uint16(dy)); err != nil {
		return nil, errors.WithStack(err)
	}

	if err := binary.Write(out, binary.BigEndian, uint32(ySize)); err != nil {
		return nil, errors.WithStack(err)
	}
	for _, b := range bufY {
		if err := binary.Write(out, binary.BigEndian, uint16(b.Len())); err != nil {
			return nil, errors.WithStack(err)
		}
		if _, err := out.ReadFrom(b); err != nil {
			return nil, errors.WithStack(err)
		}
	}
	if err := binary.Write(out, binary.BigEndian, uint32(cbSize)); err != nil {
		return nil, errors.WithStack(err)
	}
	for _, b := range bufCb {
		if err := binary.Write(out, binary.BigEndian, uint16(b.Len())); err != nil {
			return nil, errors.WithStack(err)
		}
		if _, err := out.ReadFrom(b); err != nil {
			return nil, errors.WithStack(err)
		}
	}
	if err := binary.Write(out, binary.BigEndian, uint32(crSize)); err != nil {
		return nil, errors.WithStack(err)
	}
	for _, b := range bufCr {
		if err := binary.Write(out, binary.BigEndian, uint16(b.Len())); err != nil {
			return nil, errors.WithStack(err)
		}
		if _, err := out.ReadFrom(b); err != nil {
			return nil, errors.WithStack(err)
		}
	}

	return out.Bytes(), nil
}

func encode(img *image.YCbCr, maxbitrate int) ([]byte, []byte, []byte, error) {
	img16 := YCbCrToImage16(img)
	layer2, nextImg, err := encodeLayer(img16, maxbitrate)
	if err != nil {
		return nil, nil, nil, errors.WithStack(err)
	}

	layer1, nextImg2, err := encodeLayer(nextImg, maxbitrate)
	if err != nil {
		return nil, nil, nil, errors.WithStack(err)
	}

	baseYCbCr := nextImg2.ToYCbCr()
	layer0, err := encodeBase(baseYCbCr, maxbitrate)
	if err != nil {
		return nil, nil, nil, errors.WithStack(err)
	}

	return layer0, layer1, layer2, nil
}