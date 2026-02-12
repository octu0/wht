package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"io"

	"github.com/pkg/errors"
)

func toInt16(u uint16) int16 {
	return int16(u>>1) ^ -int16(u&1)
}

func blockDecode(rr *RiceReader[uint16], size uint16) ([][]int16, error) {
	data := make([][]int16, size)
	for y := uint16(0); y < size; y += 1 {
		tmp := make([]int16, size)
		for x := uint16(0); x < size; x += 1 {
			v, err := rr.Read(k)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			tmp[x] = toInt16(v)
		}
		data[y] = tmp
	}
	return data, nil
}

func invertLayer(in io.Reader, ll [][]int16, size uint16) ([][]int16, error) {
	scaleU8 := uint8(0)
	if err := binary.Read(in, binary.BigEndian, &scaleU8); err != nil {
		return nil, errors.WithStack(err)
	}
	scale := int(scaleU8)

	rr := NewRiceReader[uint16](NewBitReader(in))

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

func invertFull(in io.Reader, size uint16) ([][]int16, error) {
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
type getLLFunc func(x, y uint16, size uint16, prediction int16) [][]int16

func invertLayerFunc(in io.Reader, w, h uint16, size uint16, predict predictFunc, setRow setRowFunc, getLL getLLFunc) ([][]int16, int16, error) {
	prediction := predict(w, h, size)
	ll := getLL(w, h, size, prediction)
	planes, err := invertLayer(in, ll, size)
	if err != nil {
		return nil, 0, errors.WithStack(err)
	}

	for i := uint16(0); i < size; i += 1 {
		setRow(w, h+i, size, planes[i], prediction)
	}
	return planes, prediction, nil
}

func invertBaseFunc(in io.Reader, w, h uint16, size uint16, predict predictFunc, setRow setRowFunc) ([][]int16, int16, error) {
	prediction := predict(w, h, size)
	planes, err := invertFull(in, size)
	if err != nil {
		return nil, 0, errors.WithStack(err)
	}

	for i := uint16(0); i < size; i += 1 {
		setRow(w, h+i, size, planes[i], prediction)
	}
	return planes, prediction, nil
}

func decodeLayer(r io.Reader, prev *Image16, size uint16) (*Image16, error) {
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
		if _, err := io.ReadFull(r, buf); err != nil {
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
		if _, err := io.ReadFull(r, buf); err != nil {
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
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, errors.WithStack(err)
		}
		crBufs[i] = buf
	}

	sub := NewImage16(dx, dy)
	tmp := newImagePredictor(image.Rect(0, 0, int(dx), int(dy)))
	for h := uint16(0); h < dy; h += size {
		for w := uint16(0); w < dx; w += size {
			data := yBufs[0]
			yBufs = yBufs[1:]
			in := bytes.NewReader(data)

			ll, prediction, err := invertLayerFunc(in, w, h, size, tmp.PredictY, tmp.UpdateY, func(x, y, sz uint16, prediction int16) [][]int16 {
				plane := prev.GetY(x/2, y/2, sz/2)
				for i := range plane {
					for j := range plane[i] {
						plane[i][j] -= prediction
					}
				}
				return plane
			})
			if err != nil {
				return nil, errors.WithStack(err)
			}
			sub.UpdateY(ll, prediction, w, h, size)
		}
	}

	for h := uint16(0); h < dy/2; h += size {
		for w := uint16(0); w < dx/2; w += size {
			data := cbBufs[0]
			cbBufs = cbBufs[1:]
			in := bytes.NewReader(data)

			ll, prediction, err := invertLayerFunc(in, w, h, size, tmp.PredictCb, tmp.UpdateCb, func(x, y, sz uint16, prediction int16) [][]int16 {
				plane := prev.GetCb(x/2, y/2, sz/2)
				for i := range plane {
					for j := range plane[i] {
						plane[i][j] -= prediction
					}
				}
				return plane
			})
			if err != nil {
				return nil, errors.WithStack(err)
			}
			sub.UpdateCb(ll, prediction, w, h, size)
		}
	}

	for h := uint16(0); h < dy/2; h += size {
		for w := uint16(0); w < dx/2; w += size {
			data := crBufs[0]
			crBufs = crBufs[1:]
			in := bytes.NewReader(data)

			ll, prediction, err := invertLayerFunc(in, w, h, size, tmp.PredictCr, tmp.UpdateCr, func(x, y, sz uint16, prediction int16) [][]int16 {
				plane := prev.GetCr(x/2, y/2, sz/2)
				for i := range plane {
					for j := range plane[i] {
						plane[i][j] -= prediction
					}
				}
				return plane
			})
			if err != nil {
				return nil, errors.WithStack(err)
			}
			sub.UpdateCr(ll, prediction, w, h, size)
		}
	}

	return sub, nil
}

func decodeBase(r io.Reader, size uint16) (*Image16, error) {
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
		if _, err := io.ReadFull(r, buf); err != nil {
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
		if _, err := io.ReadFull(r, buf); err != nil {
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
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, errors.WithStack(err)
		}
		crBufs[i] = buf
	}

	sub := NewImage16(dx, dy)
	tmp := newImagePredictor(image.Rect(0, 0, int(dx), int(dy)))
	for h := uint16(0); h < dy; h += size {
		for w := uint16(0); w < dx; w += size {
			data := yBufs[0]
			yBufs = yBufs[1:]
			in := bytes.NewReader(data)

			ll, prediction, err := invertBaseFunc(in, w, h, size, tmp.PredictY, tmp.UpdateY)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			sub.UpdateY(ll, prediction, w, h, size)
		}
	}

	for h := uint16(0); h < dy/2; h += size {
		for w := uint16(0); w < dx/2; w += size {
			data := cbBufs[0]
			cbBufs = cbBufs[1:]
			in := bytes.NewReader(data)

			ll, prediction, err := invertBaseFunc(in, w, h, size, tmp.PredictCb, tmp.UpdateCb)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			sub.UpdateCb(ll, prediction, w, h, size)
		}
	}

	for h := uint16(0); h < dy/2; h += size {
		for w := uint16(0); w < dx/2; w += size {
			data := crBufs[0]
			crBufs = crBufs[1:]
			in := bytes.NewReader(data)

			ll, prediction, err := invertBaseFunc(in, w, h, size, tmp.PredictCr, tmp.UpdateCr)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			sub.UpdateCr(ll, prediction, w, h, size)
		}
	}

	return sub, nil
}

func decode(r io.Reader) (*image.YCbCr, error) {
	buf := bytes.NewBuffer(nil)

	data0Size := uint32(0)
	if err := binary.Read(r, binary.BigEndian, &data0Size); err != nil {
		return nil, errors.WithStack(err)
	}
	if _, err := io.CopyN(buf, r, int64(data0Size)); err != nil {
		return nil, errors.WithStack(err)
	}
	layer0, err := decodeBase(bytes.NewReader(buf.Bytes()), 8)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	buf.Reset()

	data1Size := uint32(0)
	if err := binary.Read(r, binary.BigEndian, &data1Size); err != nil {
		return nil, errors.WithStack(err)
	}
	if _, err := io.CopyN(buf, r, int64(data1Size)); err != nil {
		return nil, errors.WithStack(err)
	}
	layer1, err := decodeLayer(bytes.NewReader(buf.Bytes()), layer0, 16)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	buf.Reset()

	data2Size := uint32(0)
	if err := binary.Read(r, binary.BigEndian, &data2Size); err != nil {
		return nil, errors.WithStack(err)
	}
	if _, err := io.CopyN(buf, r, int64(data2Size)); err != nil {
		return nil, errors.WithStack(err)
	}
	layer2, err := decodeLayer(bytes.NewReader(buf.Bytes()), layer1, 32)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	buf.Reset()

	return layer2.ToYCbCr(), nil
}
