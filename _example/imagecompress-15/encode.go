package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"io"

	"github.com/pkg/errors"
)

const (
	k uint8 = 1
)

func toUint16(n int16) uint16 {
	return uint16((n << 1) ^ (n >> 15))
}

func blockEncode(rw *RiceWriter[uint16], block [][]int16, size uint16) error {
	for y := uint16(0); y < size; y += 1 {
		for x := uint16(0); x < size; x += 1 {
			if err := rw.Write(toUint16(block[y][x]), k); err != nil {
				return errors.WithStack(err)
			}
		}
	}
	return nil
}

func transform(out io.Writer, data [][]int16, size uint16, scale int, isChroma bool) error {
	sub := dwt2d(data, size)

	quantizeLow(sub.LL, sub.Size, scale)
	quantizeMid(sub.HL, sub.Size, scale)
	quantizeMid(sub.LH, sub.Size, scale)
	quantizeHigh(sub.HH, sub.Size, scale)

	if err := binary.Write(out, binary.BigEndian, uint8(scale)); err != nil {
		return errors.WithStack(err)
	}

	rw := NewRiceWriter[uint16](NewBitWriter(out))
	if err := blockEncode(rw, sub.LL, sub.Size); err != nil {
		return errors.WithStack(err)
	}
	if err := blockEncode(rw, sub.HL, sub.Size); err != nil {
		return errors.WithStack(err)
	}
	if err := blockEncode(rw, sub.LH, sub.Size); err != nil {
		return errors.WithStack(err)
	}
	if err := blockEncode(rw, sub.HH, sub.Size); err != nil {
		return errors.WithStack(err)
	}
	if err := rw.Flush(); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

type predictFunc func(x, y uint16, size uint16) int16
type updatePredictFunc func(x, y uint16, size uint16, rows []int16, prediction int16)

func transformHandlerFunc(w, h uint16, size uint16, predict predictFunc, updatePredict updatePredictFunc, scale *scale, scaleVal int, isChroma bool) (*bytes.Buffer, error) {
	prediction := predict(w, h, size)
	rows, localScale := scale.Rows(w, h, size, prediction, scaleVal)

	data := bytes.NewBuffer(make([]byte, 0, size*size))
	if err := transform(data, rows, size, localScale, isChroma); err != nil {
		return nil, errors.WithStack(err)
	}

	// Local Reconstruction: DWTはブロック単位なので、一度デコードしてから一括更新
	planes, err := invert(bytes.NewReader(data.Bytes()), size)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	for i := uint16(0); i < size; i += 1 {
		updatePredict(w, h+i, size, planes[i], prediction)
	}
	return data, nil
}

func encode(img *image.YCbCr, maxbitrate int) ([]byte, error) {
	bufY := make([]*bytes.Buffer, 0)
	bufCb := make([]*bytes.Buffer, 0)
	bufCr := make([]*bytes.Buffer, 0)

	r := newImageReader(img)
	b := img.Bounds()
	dx, dy := uint16(b.Dx()), uint16(b.Dy())

	scaleVal := 1
	scaleY := newScale(r.RowY)
	scaler := newRateController(maxbitrate, dx, dy)
	tmp := newImagePredictor(img.Bounds())
	for h := uint16(0); h < dy; h += 32 {
		for w := uint16(0); w < dx; w += 32 {
			data, err := transformHandlerFunc(w, h, 32, tmp.PredictY, tmp.UpdateY, scaleY, scaleVal, false)
			if err != nil {
				panic(fmt.Sprintf("%+v", err))
			}
			bufY = append(bufY, data)
			scaleVal = scaler.CalcScale(data.Len()*8, 32*32)
		}
	}

	scaleCb := newScale(r.RowCb)
	for h := uint16(0); h < dy/2; h += 32 {
		for w := uint16(0); w < dx/2; w += 32 {
			data, err := transformHandlerFunc(w, h, 32, tmp.PredictCb, tmp.UpdateCb, scaleCb, scaleVal, true)
			if err != nil {
				panic(fmt.Sprintf("%+v", err))
			}
			bufCb = append(bufCb, data)
			scaleVal = scaler.CalcScale(data.Len()*8, 32*32)
		}
	}

	scaleCr := newScale(r.RowCr)
	for h := uint16(0); h < dy/2; h += 32 {
		for w := uint16(0); w < dx/2; w += 32 {
			data, err := transformHandlerFunc(w, h, 32, tmp.PredictCr, tmp.UpdateCr, scaleCr, scaleVal, true)
			if err != nil {
				panic(fmt.Sprintf("%+v", err))
			}
			bufCr = append(bufCr, data)
			scaleVal = scaler.CalcScale(data.Len()*8, 32*32)
		}
	}

	out := bytes.NewBuffer(nil)

	if err := binary.Write(out, binary.BigEndian, dx); err != nil {
		return nil, errors.WithStack(err)
	}
	if err := binary.Write(out, binary.BigEndian, dy); err != nil {
		return nil, errors.WithStack(err)
	}

	if err := binary.Write(out, binary.BigEndian, uint16(len(bufY))); err != nil {
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
	if err := binary.Write(out, binary.BigEndian, uint16(len(bufCb))); err != nil {
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
	if err := binary.Write(out, binary.BigEndian, uint16(len(bufCr))); err != nil {
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
