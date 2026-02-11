package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"io"
	"unsafe"

	"github.com/pkg/errors"
)

func blockDecode(in io.Reader, size uint16) ([]int16, error) {
	rw := NewRiceReader[uint16](NewBitReader(in))
	data, err := blockRLEDecode(rw, size*size)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	// uint16 -> int16
	block := unsafe.Slice((*int16)(unsafe.Pointer(&data[0])), len(data))
	return block, nil
}

func invert(in io.Reader, size uint16) ([][]int16, error) {
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

type setRowFunc func(x, y uint16, size uint16, plane []int16, prediction int16)

func invertHandlerFunc(in io.Reader, w, h uint16, size uint16, predict predictFunc, setRow setRowFunc) error {
	prediction := predict(w, h, size)
	planes, err := invert(in, size)
	if err != nil {
		return errors.WithStack(err)
	}

	for i := uint16(0); i < size; i += 1 {
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

	ip := newImagePredictor(image.Rect(0, 0, int(dx), int(dy)))
	for h := uint16(0); h < dy; h += 32 {
		for w := uint16(0); w < dx; w += 32 {
      data := yBufs[0]
      yBufs = yBufs[1:]
      in := bytes.NewReader(data)

      if err := invertHandlerFunc(in, w, h, 32, ip.PredictY, ip.UpdateY); err != nil {
        return nil, errors.WithStack(err)
			}
		}
	}

	for h := uint16(0); h < dy/2; h += 32 {
		for w := uint16(0); w < dx/2; w += 32 {
      data := cbBufs[0]
      cbBufs = cbBufs[1:]
      in := bytes.NewReader(data)

      if err := invertHandlerFunc(in, w, h, 32, ip.PredictCb, ip.UpdateCb); err != nil {
        return nil, errors.WithStack(err)
			}
		}
	}

	for h := uint16(0); h < dy/2; h += 32 {
		for w := uint16(0); w < dx/2; w += 32 {
      data := crBufs[0]
      crBufs = crBufs[1:]
      in := bytes.NewReader(data)

      if err := invertHandlerFunc(in, w, h, 32, ip.PredictCr, ip.UpdateCr); err != nil {
        return nil, errors.WithStack(err)
			}
		}
	}

	return ip.img, nil
}
