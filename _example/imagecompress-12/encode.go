package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"io"
	"unsafe"

	"github.com/octu0/runlength"
	"github.com/pkg/errors"
)

const (
	// BitrateBudgetRatio specifies the allocation of the total bitrate to each layer.
	// Total roughly exceeds 100% to allow flexibility, handled by RateController.
	Layer2BitrateRatio = 0.63   // 63/100
	Layer1BitrateRatio = 0.315  // 63/200
	Layer0BitrateRatio = 0.1575 // 63/400

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

func transformBase(out io.Writer, data [][]int16, size, scale int) error {
	dwtBlock2Level(data, size)
	return encodeBlockData(out, data, size, scale)
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
	out, err := serializeStreams(dx, dy, bufY, bufCb, bufCr, nil, nil, nil)
	if err != nil {
		return nil, nil, err
	}
	return out, nextImg, nil
}

func transformHandlerFunc(w, h int, size int, predict predictFunc, updatePredict updatePredictFunc, scale *scale, scaleVal int, isChroma bool) (*bytes.Buffer, error) {
	prediction := predict(w, h, size)
	rows, localScale := scale.Rows(w, h, size, prediction, scaleVal)

	data := bytes.NewBuffer(make([]byte, 0, size*size))
	if err := transformBase(data, rows, size, localScale); err != nil {
		return nil, errors.WithStack(err)
	}

	// Local Reconstruction
	planes, err := invert(bytes.NewReader(data.Bytes()), size)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	for i := 0; i < size; i += 1 {
		updatePredict(w, h+i, size, planes[i], prediction)
	}
	return data, nil
}

// encodeBase はベースレイヤー(Layer0)をエンコードします
func encodeBase(img *image.YCbCr, maxbitrate int) ([]byte, error) {
	bufY := make([]*bytes.Buffer, 0)
	bufCb := make([]*bytes.Buffer, 0)
	bufCr := make([]*bytes.Buffer, 0)

	mbYSeq := make([]uint8, 0)
	mbCbSeq := make([]uint8, 0)
	mbCrSeq := make([]uint8, 0)

	r := newImageReader(img)
	b := img.Bounds()
	dx, dy := b.Dx(), b.Dy()

	scaleVal := 1
	scaleY := newScale(r.RowY)
	scaler := newRateController(maxbitrate, dx, dy)
	tmp := newImagePredictor(img.Bounds())

	for h := 0; h < dy; h += 32 {
		for w := 0; w < dx; w += 32 {
			mb := r.CalcMacroblockY(w, h)
			parts := mbPartitoins[mb]

			for _, p := range parts {
				blockX := w + p.X
				blockY := h + p.Y
				data, err := transformHandlerFunc(blockX, blockY, p.Size, tmp.PredictY, tmp.UpdateY, scaleY, scaleVal, false)
				if err != nil {
					panic(fmt.Sprintf("%+v", err))
				}
				bufY = append(bufY, data)
				scaleVal = scaler.CalcScale(data.Len()*8, p.Size*p.Size)
			}
			mbYSeq = append(mbYSeq, uint8(mb))
		}
	}

	scaleCb := newScale(r.RowCb)
	for h := 0; h < dy/2; h += 16 {
		for w := 0; w < dx/2; w += 16 {
			mb := r.CalcMacroblockCb(w, h)
			parts := mbPartitoins[mb]

			for _, p := range parts {
				blockX := w + p.X
				blockY := h + p.Y
				data, err := transformHandlerFunc(blockX, blockY, p.Size, tmp.PredictCb, tmp.UpdateCb, scaleCb, scaleVal, true)
				if err != nil {
					panic(fmt.Sprintf("%+v", err))
				}
				bufCb = append(bufCb, data)
				scaleVal = scaler.CalcScale(data.Len()*8, p.Size*p.Size)
			}
			mbCbSeq = append(mbCbSeq, uint8(mb))
		}
	}

	scaleCr := newScale(r.RowCr)
	for h := 0; h < dy/2; h += 16 {
		for w := 0; w < dx/2; w += 16 {
			mb := r.CalcMacroblockCr(w, h)
			parts := mbPartitoins[mb]

			for _, p := range parts {
				blockX := w + p.X
				blockY := h + p.Y
				data, err := transformHandlerFunc(blockX, blockY, p.Size, tmp.PredictCr, tmp.UpdateCr, scaleCr, scaleVal, true)
				if err != nil {
					panic(fmt.Sprintf("%+v", err))
				}
				bufCr = append(bufCr, data)
				scaleVal = scaler.CalcScale(data.Len()*8, p.Size*p.Size)
			}
			mbCrSeq = append(mbCrSeq, uint8(mb))
		}
	}

	return serializeStreams(dx, dy, bufY, bufCb, bufCr, mbYSeq, mbCbSeq, mbCrSeq)
}

func serializeStreams(dx, dy int, bufY, bufCb, bufCr []*bytes.Buffer, mbYSeq, mbCbSeq, mbCrSeq []uint8) ([]byte, error) {
	mbYSeqRLEBuf := bytes.NewBuffer(nil)
	if mbYSeq != nil {
		if err := runlength.NewEncoder(mbYSeqRLEBuf).Encode(mbYSeq); err != nil {
			return nil, errors.WithStack(err)
		}
	}
	mbCbSeqRLEBuf := bytes.NewBuffer(nil)
	if mbCbSeq != nil {
		if err := runlength.NewEncoder(mbCbSeqRLEBuf).Encode(mbCbSeq); err != nil {
			return nil, errors.WithStack(err)
		}
	}
	mbCrSeqRLEBuf := bytes.NewBuffer(nil)
	if mbCrSeq != nil {
		if err := runlength.NewEncoder(mbCrSeqRLEBuf).Encode(mbCrSeq); err != nil {
			return nil, errors.WithStack(err)
		}
	}

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

	out := bytes.NewBuffer(make([]byte, 0, 2+2+ySize+cbSize+crSize+len(mbYSeq)+len(mbCbSeq)+len(mbCrSeq)))

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

	if mbYSeq != nil {
		if err := binary.Write(out, binary.BigEndian, uint32(mbYSeqRLEBuf.Len())); err != nil {
			return nil, errors.WithStack(err)
		}
		if _, err := out.ReadFrom(mbYSeqRLEBuf); err != nil {
			return nil, errors.WithStack(err)
		}
	}
	if mbCbSeq != nil {
		if err := binary.Write(out, binary.BigEndian, uint32(mbCbSeqRLEBuf.Len())); err != nil {
			return nil, errors.WithStack(err)
		}
		if _, err := out.ReadFrom(mbCbSeqRLEBuf); err != nil {
			return nil, errors.WithStack(err)
		}
	}
	if mbCrSeq != nil {
		if err := binary.Write(out, binary.BigEndian, uint32(mbCrSeqRLEBuf.Len())); err != nil {
			return nil, errors.WithStack(err)
		}
		if _, err := out.ReadFrom(mbCrSeqRLEBuf); err != nil {
			return nil, errors.WithStack(err)
		}
	}

	return out.Bytes(), nil
}

type predictFunc func(x, y int, size int) int16
type updatePredictFunc func(x, y int, size int, rows []int16, prediction int16)

func encode(img *image.YCbCr, maxbitrate int) ([]byte, []byte, []byte, error) {
	img16 := YCbCrToImage16(img)
	layer2, nextImg, err := encodeLayer(img16, int(float64(maxbitrate)*Layer2BitrateRatio))
	if err != nil {
		return nil, nil, nil, errors.WithStack(err)
	}

	layer1, nextImg2, err := encodeLayer(nextImg, int(float64(maxbitrate)*Layer1BitrateRatio))
	if err != nil {
		return nil, nil, nil, errors.WithStack(err)
	}

	baseYCbCr := nextImg2.ToYCbCr()
	layer0, err := encodeBase(baseYCbCr, int(float64(maxbitrate)*Layer0BitrateRatio))
	if err != nil {
		return nil, nil, nil, errors.WithStack(err)
	}

	return layer0, layer1, layer2, nil
}
