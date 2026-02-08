package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"io"
	"unsafe"

	"github.com/octu0/runlength"
	"github.com/pkg/errors"
)

func blockDecode(in io.Reader, size int) ([]int16, error) {
	rw := NewRiceReader[uint16](NewBitReader(in))
	data, err := blockRLEDecode(rw, size*size)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	// uint16 -> int16
	block := unsafe.Slice((*int16)(unsafe.Pointer(&data[0])), len(data))
	return block, nil
}

func invert(in io.Reader, size int) ([][]int16, error) {
	scale := uint8(0)
	if err := binary.Read(in, binary.BigEndian, &scale); err != nil {
		return nil, errors.WithStack(err)
	}

	zigzag, err := blockDecode(in, size)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	block := Unzigzag(zigzag, size)

	dequantizeBlock(block, size, int(scale))
	invDwtBlock2Level(block, size)

	return block, nil
}

type setRowFunc func(x, y int, size int, plane []int16, prediction int16)

func invertHandlerBase(in io.Reader, w, h int, size int, predict predictFunc, setRow setRowFunc) error {
	prediction := predict(w, h, size)
	planes, err := invert(in, size)
	if err != nil {
		return errors.WithStack(err)
	}

	for i := 0; i < size; i += 1 {
		setRow(w, h+i, size, planes[i], prediction)
	}
	return nil
}

func decodeSubband(in [][]byte, plane []int16, width, height int, startX, startY, endX, endY int) ([][]byte, error) {
	blockSize := EncBlockSize
	for y := startY; y < endY; y += blockSize {
		for x := startX; x < endX; x += blockSize {
			if len(in) == 0 {
				return nil, errors.New("not enough blocks")
			}
			blockData := in[0]
			in = in[1:]

			// Read Scale
			scale := uint8(0)
			r := bytes.NewReader(blockData)
			if err := binary.Read(r, binary.BigEndian, &scale); err != nil {
				return nil, errors.WithStack(err)
			}

			zigzag, err := blockDecode(r, blockSize)
			if err != nil {
				return nil, err
			}
			block := Unzigzag(zigzag, blockSize)
			dequantizeBlock(block, blockSize, int(scale))

			// Copy to plane
			for i := 0; i < blockSize; i += 1 {
				for j := 0; j < blockSize; j += 1 {
					px, py := x+j, y+i
					if px < width && py < height {
						plane[(py*width)+px] = block[i][j]
					}
				}
			}
		}
	}
	return in, nil
}

// decodeLayer は1つの解像度レイヤーをデコードし、合成された画像を返します
// prevImg は1つ下の解像度（LL成分）の画像です
func decodeLayer(data []byte, prevImg *Image16) (*Image16, error) {
	r := bytes.NewReader(data)

	// Read Header
	dx, dy := uint16(0), uint16(0)
	if err := binary.Read(r, binary.BigEndian, &dx); err != nil {
		return nil, errors.WithStack(err)
	}
	if err := binary.Read(r, binary.BigEndian, &dy); err != nil {
		return nil, errors.WithStack(err)
	}

	ySize := uint32(0)
	if err := binary.Read(r, binary.BigEndian, &ySize); err != nil {
		return nil, errors.WithStack(err)
	}
	yBufs := make([][]byte, 0)
	for i := uint32(0); i < ySize; {
		blockLen := uint16(0)
		if err := binary.Read(r, binary.BigEndian, &blockLen); err != nil {
			return nil, errors.WithStack(err)
		}
		buf := make([]byte, blockLen)
		if _, err := r.Read(buf); err != nil {
			return nil, errors.WithStack(err)
		}
		yBufs = append(yBufs, buf)
		i += uint32(blockLen)
	}

	cbSize := uint32(0)
	if err := binary.Read(r, binary.BigEndian, &cbSize); err != nil {
		return nil, errors.WithStack(err)
	}
	cbBufs := make([][]byte, 0)
	for i := uint32(0); i < cbSize; {
		blockLen := uint16(0)
		if err := binary.Read(r, binary.BigEndian, &blockLen); err != nil {
			return nil, errors.WithStack(err)
		}
		buf := make([]byte, blockLen)
		if _, err := r.Read(buf); err != nil {
			return nil, errors.WithStack(err)
		}
		cbBufs = append(cbBufs, buf)
		i += uint32(blockLen)
	}

	crSize := uint32(0)
	if err := binary.Read(r, binary.BigEndian, &crSize); err != nil {
		return nil, errors.WithStack(err)
	}
	crBufs := make([][]byte, 0)
	for i := uint32(0); i < crSize; {
		blockLen := uint16(0)
		if err := binary.Read(r, binary.BigEndian, &blockLen); err != nil {
			return nil, errors.WithStack(err)
		}
		buf := make([]byte, blockLen)
		if _, err := r.Read(buf); err != nil {
			return nil, errors.WithStack(err)
		}
		crBufs = append(crBufs, buf)
		i += uint32(blockLen)
	}

	// Skip sequence numbers
	mbSeqSize := uint32(0)
	binary.Read(r, binary.BigEndian, &mbSeqSize)
	r.Seek(int64(mbSeqSize), io.SeekCurrent)

	binary.Read(r, binary.BigEndian, &mbSeqSize)
	r.Seek(int64(mbSeqSize), io.SeekCurrent)

	binary.Read(r, binary.BigEndian, &mbSeqSize)
	r.Seek(int64(mbSeqSize), io.SeekCurrent)

	// Decode
	currentImg := NewImage16(int(dx), int(dy))

	// Y Plane
	w, h := int(dx), int(dy)
	nextW, nextH := w/2, h/2

	// Copy LL from prevImg
	for y := 0; y < nextH; y += 1 {
		for x := 0; x < nextW; x += 1 {
			py := y
			if prevImg.Height <= py {
				py = prevImg.Height - 1
			}
			px := x
			if prevImg.Width <= px {
				px = prevImg.Width - 1
			}

			currentImg.Y[(y*w)+x] = prevImg.Y[prevImg.YOffset(px, py)]
		}
	}

	// Decode HL (Right-Top)
	if nextBufs, err := decodeSubband(yBufs, currentImg.Y, w, h, nextW, 0, w, nextH); err != nil {
		return nil, errors.WithStack(err)
	} else {
		yBufs = nextBufs
	}
	// Decode LH (Left-Bottom)
	if nextBufs, err := decodeSubband(yBufs, currentImg.Y, w, h, 0, nextH, nextW, h); err != nil {
		return nil, errors.WithStack(err)
	} else {
		yBufs = nextBufs
	}
	// Decode HH (Right-Bottom)
	if nextBufs, err := decodeSubband(yBufs, currentImg.Y, w, h, nextW, nextH, w, h); err != nil {
		return nil, errors.WithStack(err)
	} else {
		yBufs = nextBufs
	}

	// InvDWT
	InvDWTPlane(currentImg.Y, w, h)

	// Chroma
	cw, ch := w/2, h/2
	nextCW, nextCH := cw/2, ch/2

	// Copy Cb LL
	for y := 0; y < nextCH; y += 1 {
		for x := 0; x < nextCW; x += 1 {
			py := y
			if prevImg.Height/2 <= py {
				py = prevImg.Height/2 - 1
			}
			px := x
			if prevImg.Width/2 <= px {
				px = prevImg.Width/2 - 1
			}
			currentImg.Cb[(y*cw)+x] = prevImg.Cb[prevImg.COffset(px, py)]
		}
	}
	if nextBufs, err := decodeSubband(cbBufs, currentImg.Cb, cw, ch, nextCW, 0, cw, nextCH); err != nil {
		return nil, errors.WithStack(err)
	} else {
		cbBufs = nextBufs
	}
	if nextBufs, err := decodeSubband(cbBufs, currentImg.Cb, cw, ch, 0, nextCH, nextCW, ch); err != nil {
		return nil, errors.WithStack(err)
	} else {
		cbBufs = nextBufs
	}
	if nextBufs, err := decodeSubband(cbBufs, currentImg.Cb, cw, ch, nextCW, nextCH, cw, ch); err != nil {
		return nil, errors.WithStack(err)
	} else {
		cbBufs = nextBufs
	}
	InvDWTPlane(currentImg.Cb, cw, ch)

	// Copy Cr LL
	for y := 0; y < nextCH; y += 1 {
		for x := 0; x < nextCW; x += 1 {
			py := y
			if prevImg.Height/2 <= py {
				py = prevImg.Height/2 - 1
			}
			px := x
			if prevImg.Width/2 <= px {
				px = prevImg.Width/2 - 1
			}
			currentImg.Cr[(y*cw)+x] = prevImg.Cr[prevImg.COffset(px, py)]
		}
	}
	if nextBufs, err := decodeSubband(crBufs, currentImg.Cr, cw, ch, nextCW, 0, cw, nextCH); err != nil {
		return nil, errors.WithStack(err)
	} else {
		crBufs = nextBufs
	}
	if nextBufs, err := decodeSubband(crBufs, currentImg.Cr, cw, ch, 0, nextCH, nextCW, ch); err != nil {
		return nil, errors.WithStack(err)
	} else {
		crBufs = nextBufs
	}
	if nextBufs, err := decodeSubband(crBufs, currentImg.Cr, cw, ch, nextCW, nextCH, cw, ch); err != nil {
		return nil, errors.WithStack(err)
	} else {
		crBufs = nextBufs
	}
	InvDWTPlane(currentImg.Cr, cw, ch)

	return currentImg, nil
}

// decodeBase はベースレイヤー(Layer0)をデコードします
func decodeBase(r io.Reader) (*image.YCbCr, error) {
	dx, dy := uint16(0), uint16(0)
	if err := binary.Read(r, binary.BigEndian, &dx); err != nil {
		return nil, errors.WithStack(err)
	}
	if err := binary.Read(r, binary.BigEndian, &dy); err != nil {
		return nil, errors.WithStack(err)
	}

	ySize := uint32(0)
	if err := binary.Read(r, binary.BigEndian, &ySize); err != nil {
		return nil, errors.WithStack(err)
	}

	yBufs := make([][]byte, 0)
	for i := uint32(0); i < ySize; {
		blockLen := uint16(0)
		if err := binary.Read(r, binary.BigEndian, &blockLen); err != nil {
			return nil, errors.WithStack(err)
		}
		buf := make([]byte, blockLen)
		if _, err := r.Read(buf); err != nil {
			return nil, errors.WithStack(err)
		}
		yBufs = append(yBufs, buf)
		i += uint32(blockLen)
	}

	cbSize := uint32(0)
	if err := binary.Read(r, binary.BigEndian, &cbSize); err != nil {
		return nil, errors.WithStack(err)
	}

	cbBufs := make([][]byte, 0)
	for i := uint32(0); i < cbSize; {
		blockLen := uint16(0)
		if err := binary.Read(r, binary.BigEndian, &blockLen); err != nil {
			return nil, errors.WithStack(err)
		}
		buf := make([]byte, blockLen)
		if _, err := r.Read(buf); err != nil {
			return nil, errors.WithStack(err)
		}
		cbBufs = append(cbBufs, buf)
		i += uint32(blockLen)
	}

	crSize := uint32(0)
	if err := binary.Read(r, binary.BigEndian, &crSize); err != nil {
		return nil, errors.WithStack(err)
	}

	crBufs := make([][]byte, 0)
	for i := uint32(0); i < crSize; {
		blockLen := uint16(0)
		if err := binary.Read(r, binary.BigEndian, &blockLen); err != nil {
			return nil, errors.WithStack(err)
		}
		buf := make([]byte, blockLen)
		if _, err := r.Read(buf); err != nil {
			return nil, errors.WithStack(err)
		}
		crBufs = append(crBufs, buf)
		i += uint32(blockLen)
	}

	mbYSeqSize := uint32(0)
	if err := binary.Read(r, binary.BigEndian, &mbYSeqSize); err != nil {
		return nil, errors.WithStack(err)
	}
	mbYSeqBuf := make([]byte, mbYSeqSize)
	if _, err := r.Read(mbYSeqBuf); err != nil {
		return nil, errors.WithStack(err)
	}

	mbCbSeqSize := uint32(0)
	if err := binary.Read(r, binary.BigEndian, &mbCbSeqSize); err != nil {
		return nil, errors.WithStack(err)
	}
	mbCbSeqBuf := make([]byte, mbCbSeqSize)
	if _, err := r.Read(mbCbSeqBuf); err != nil {
		return nil, errors.WithStack(err)
	}
	mbCrSeqSize := uint32(0)
	if err := binary.Read(r, binary.BigEndian, &mbCrSeqSize); err != nil {
		return nil, errors.WithStack(err)
	}
	mbCrSeqBuf := make([]byte, mbCrSeqSize)
	if _, err := r.Read(mbCrSeqBuf); err != nil {
		return nil, errors.WithStack(err)
	}

	mbYSeq, err := runlength.NewDecoder().Decode(bytes.NewReader(mbYSeqBuf))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	mbCbSeq, err := runlength.NewDecoder().Decode(bytes.NewReader(mbCbSeqBuf))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	mbCrSeq, err := runlength.NewDecoder().Decode(bytes.NewReader(mbCrSeqBuf))
	if err != nil {
		return nil, errors.WithStack(err)
	}

	ip := newImagePredictor(image.Rect(0, 0, int(dx), int(dy)))
	for h := uint16(0); h < dy; h += 32 {
		for w := uint16(0); w < dx; w += 32 {
			mb := macroblock(mbYSeq[0])
			mbYSeq = mbYSeq[1:]
			parts := mbPartitoins[mb]

			for _, p := range parts {
				blockX := int(w) + p.X
				blockY := int(h) + p.Y
				data := yBufs[0]
				yBufs = yBufs[1:]
				in := bytes.NewReader(data)

				if err := invertHandlerBase(in, blockX, blockY, p.Size, ip.PredictY, ip.UpdateY); err != nil {
					return nil, errors.WithStack(err)
				}
			}
		}
	}

	for h := uint16(0); h < dy/2; h += 16 {
		for w := uint16(0); w < dx/2; w += 16 {
			mb := macroblock(mbCbSeq[0])
			mbCbSeq = mbCbSeq[1:]
			parts := mbPartitoins[mb]

			for _, p := range parts {
				blockX := int(w) + p.X
				blockY := int(h) + p.Y
				data := cbBufs[0]
				cbBufs = cbBufs[1:]
				in := bytes.NewReader(data)

				if err := invertHandlerBase(in, blockX, blockY, p.Size, ip.PredictCb, ip.UpdateCb); err != nil {
					return nil, errors.WithStack(err)
				}
			}
		}
	}

	for h := uint16(0); h < dy/2; h += 16 {
		for w := uint16(0); w < dx/2; w += 16 {
			mb := macroblock(mbCrSeq[0])
			mbCrSeq = mbCrSeq[1:]
			parts := mbPartitoins[mb]

			for _, p := range parts {
				blockX := int(w) + p.X
				blockY := int(h) + p.Y
				data := crBufs[0]
				crBufs = crBufs[1:]
				in := bytes.NewReader(data)

				if err := invertHandlerBase(in, blockX, blockY, p.Size, ip.PredictCr, ip.UpdateCr); err != nil {
					return nil, errors.WithStack(err)
				}
			}
		}
	}

	return ip.img, nil
}

func decode(layers ...[]byte) (*image.YCbCr, error) {
	if len(layers) == 0 {
		return nil, errors.New("no layers")
	}

	// Layer 0: Base
	baseImg, err := decodeBase(bytes.NewReader(layers[0]))
	if err != nil {
		return nil, errors.WithStack(err)
	}

	if len(layers) == 1 {
		return baseImg, nil
	}

	// Layer 1: Medium
	baseImg16 := YCbCrToImage16(baseImg)
	layer1Img, err := decodeLayer(layers[1], baseImg16)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	if len(layers) == 2 {
		return layer1Img.ToYCbCr(), nil
	}

	// Layer 2: High
	layer2Img, err := decodeLayer(layers[2], layer1Img)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return layer2Img.ToYCbCr(), nil
}
