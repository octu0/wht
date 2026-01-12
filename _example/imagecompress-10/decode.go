package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"io"
	"unsafe"

	"github.com/octu0/runlength"
	"github.com/octu0/wht"
	"github.com/pkg/errors"
)

func dcDecode(in io.Reader, size int) ([]int16, error) {
	dc := make([]int16, size)
	for i := 0; i < size; i += 1 {
		if err := binary.Read(in, binary.BigEndian, &dc[i]); err != nil {
			return nil, errors.WithStack(err)
		}
	}
	return dc, nil
}

func acDecode(in io.Reader, rows, cols int) ([]int8, error) {
	rw := NewRiceReader[uint8](NewBitReader(in))
	data, err := rleDecode(rw, rows, cols)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	// uint8 -> int8
	ac := unsafe.Slice((*int8)(unsafe.Pointer(&data[0])), len(data))
	return ac, nil
}

func invert(in io.Reader, dcQT, acQT QuantizationTable, size int) ([][]int16, error) {
	scale := uint8(0)
	if err := binary.Read(in, binary.BigEndian, &scale); err != nil {
		return nil, errors.WithStack(err)
	}

	dc, err := dcDecode(in, size)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	dcDequantize(dc, dcQT)
	wht.Invert(dc)

	zigzag, err := acDecode(in, size, size-1)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	acList := make([][]int16, size)
	acPlanes := Unzigzag(zigzag, size, size-1)
	for i := 0; i < size; i += 1 {
		ac := acDequantize(acPlanes[i], acQT, int(scale))
		acList[i] = make([]int16, size)
		acList[i][0] = dc[i]
		copy(acList[i][1:size], ac)
		wht.Invert(acList[i])
	}
	return acList, nil
}

type setRowFunc func(x, y int, size int, plane []int16, prediction int16)

func invertHandlerFunc(in io.Reader, w, h int, size int, dcQT, acQT QuantizationTable, predict predictFunc, setRow setRowFunc) error {
	prediction := predict(w, h, size)
	planes, err := invert(in, dcQT, acQT, size)
	if err != nil {
		return errors.WithStack(err)
	}

	for i := 0; i < size; i += 1 {
		setRow(w, h+i, size, planes[i], prediction)
	}
	return nil
}

func decode(r io.Reader) (*image.YCbCr, error) {
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
	fmt.Println("crSize:", crSize)

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

				qtPair := qtBySize(p.Size)
				dcQT, lumaQT := qtPair[0], qtPair[1]
				if err := invertHandlerFunc(in, blockX, blockY, p.Size, dcQT, lumaQT, ip.PredictY, ip.UpdateY); err != nil {
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

				qtPair := qtBySize(p.Size)
				dcQT, chromaQT := qtPair[0], qtPair[2]
				if err := invertHandlerFunc(in, blockX, blockY, p.Size, dcQT, chromaQT, ip.PredictCb, ip.UpdateCb); err != nil {
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

				qtPair := qtBySize(p.Size)
				dcQT, chromaQT := qtPair[0], qtPair[2]
				if err := invertHandlerFunc(in, blockX, blockY, p.Size, dcQT, chromaQT, ip.PredictCr, ip.UpdateCr); err != nil {
					return nil, errors.WithStack(err)
				}
			}
		}
	}

	ip.Deblocking()

	return ip.img, nil
}
