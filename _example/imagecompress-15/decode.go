package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"io"
	"unsafe"

	"github.com/pkg/errors"
)

func blockDecode(rr *RiceReader[uint16], size uint16) ([][]int16, error) {
	data := make([][]int16, size)
	for y := uint16(0); y < size; y += 1 {
		tmp := make([]uint16, size)
		for x := uint16(0); x < size; x += 1 {
			v, err := rr.Read(k)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			tmp[x] = v
		}
		// uint16 -> int16
		data[y] = unsafe.Slice((*int16)(unsafe.Pointer(&tmp[0])), len(tmp))
	}
	return data, nil
}

func invert(in io.Reader, size uint16) ([][]int16, error) {
	scaleU8 := uint8(0)
	if err := binary.Read(in, binary.BigEndian, &scaleU8); err != nil {
		return nil, errors.WithStack(err)
	}
	scale := int(scaleU8)

	rr := NewRiceReader[uint16](NewBitReader(in))

	ll, err := blockDecode(rr, size/2)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	hl, err := blockDecode(rr, size/2)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	lh, err := blockDecode(rr, size/2)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	hh, err := blockDecode(rr, size/2)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	dequantizeLow(ll, size/2, scale)
	dequantizeMid(hl, size/2, scale)
	dequantizeMid(lh, size/2, scale)
	dequantizeHigh(hh, size/2, scale)

	sub := Subbands{
		LL:   ll,
		HL:   hl,
		LH:   lh,
		HH:   hh,
		Size: size / 2,
	}
	return invDwt2d(sub), nil
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

	bufYLen := uint16(0)
	if err := binary.Read(r, binary.BigEndian, &bufYLen); err != nil {
		return nil, errors.WithStack(err)
	}

	yBufs := make([][]byte, bufYLen)
	for i := uint16(0); i < bufYLen; i += 1 {
		blockLen := uint16(0)
		if err := binary.Read(r, binary.BigEndian, &blockLen); err != nil {
			return nil, errors.WithStack(err)
		}
		buf := make([]byte, blockLen)
		if _, err := r.Read(buf); err != nil {
			return nil, errors.WithStack(err)
		}
		yBufs[i] = buf
	}

	bufCbLen := uint16(0)
	if err := binary.Read(r, binary.BigEndian, &bufCbLen); err != nil {
		return nil, errors.WithStack(err)
	}

	cbBufs := make([][]byte, bufCbLen)
	for i := uint16(0); i < bufCbLen; i += 1 {
		blockLen := uint16(0)
		if err := binary.Read(r, binary.BigEndian, &blockLen); err != nil {
			return nil, errors.WithStack(err)
		}
		buf := make([]byte, blockLen)
		if _, err := r.Read(buf); err != nil {
			return nil, errors.WithStack(err)
		}
		cbBufs[i] = buf
	}

	bufCrLen := uint16(0)
	if err := binary.Read(r, binary.BigEndian, &bufCrLen); err != nil {
		return nil, errors.WithStack(err)
	}

	crBufs := make([][]byte, bufCrLen)
	for i := uint16(0); i < bufCrLen; i += 1 {
		blockLen := uint16(0)
		if err := binary.Read(r, binary.BigEndian, &blockLen); err != nil {
			return nil, errors.WithStack(err)
		}
		buf := make([]byte, blockLen)
		if _, err := r.Read(buf); err != nil {
			return nil, errors.WithStack(err)
		}
		crBufs[i] = buf
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
