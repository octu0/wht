package main

import (
	"bytes"
	"encoding/binary"
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

func transform(out io.Writer, data [][]int16, size uint16, scale int) ([][]int16, error) {
	sub := dwt2d(data, size)

	quantizeMid(sub.HL, sub.Size, scale)
	quantizeMid(sub.LH, sub.Size, scale)
	quantizeHigh(sub.HH, sub.Size, scale)

	if err := binary.Write(out, binary.BigEndian, uint8(scale)); err != nil {
		return nil, errors.WithStack(err)
	}

	rw := NewRiceWriter[uint16](NewBitWriter(out))
	if err := blockEncode(rw, sub.HL, sub.Size); err != nil {
		return nil, errors.WithStack(err)
	}
	if err := blockEncode(rw, sub.LH, sub.Size); err != nil {
		return nil, errors.WithStack(err)
	}
	if err := blockEncode(rw, sub.HH, sub.Size); err != nil {
		return nil, errors.WithStack(err)
	}
	if err := rw.Flush(); err != nil {
		return nil, errors.WithStack(err)
	}
	return sub.LL, nil
}

func transformFull(out io.Writer, data [][]int16, size uint16, scale int) error {
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

func transformLayer(w, h uint16, size uint16, predict predictFunc, updatePredict updatePredictFunc, scale *scale, scaleVal int) (*bytes.Buffer, [][]int16, int16, error) {
	prediction := predict(w, h, size)
	rows, localScale := scale.Rows(w, h, size, prediction, scaleVal)

	data := bytes.NewBuffer(make([]byte, 0, size*size))
	ll, err := transform(data, rows, size, localScale)
	if err != nil {
		return nil, nil, 0, errors.WithStack(err)
	}

	// Local Reconstruction
	planes, err := invertLayer(bytes.NewReader(data.Bytes()), ll, size)
	if err != nil {
		return nil, nil, 0, errors.WithStack(err)
	}
	for i := uint16(0); i < size; i += 1 {
		updatePredict(w, h+i, size, planes[i], prediction)
	}
	return data, ll, prediction, nil
}

func transformBase(w, h uint16, size uint16, predict predictFunc, updatePredict updatePredictFunc, scale *scale, scaleVal int) (*bytes.Buffer, error) {
	prediction := predict(w, h, size)
	rows, localScale := scale.Rows(w, h, size, prediction, scaleVal)

	data := bytes.NewBuffer(make([]byte, 0, size*size))
	if err := transformFull(data, rows, size, localScale); err != nil {
		return nil, errors.WithStack(err)
	}

	// Local Reconstruction
	planes, err := invertFull(bytes.NewReader(data.Bytes()), size)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	for i := uint16(0); i < size; i += 1 {
		updatePredict(w, h+i, size, planes[i], prediction)
	}
	return data, nil
}

func encodeLayer(r *ImageReader, scaler *RateController, scaleVal int, size uint16) ([]byte, *Image16, int, error) {
	bufY := make([]*bytes.Buffer, 0)
	bufCb := make([]*bytes.Buffer, 0)
	bufCr := make([]*bytes.Buffer, 0)

	dx, dy := r.Width(), r.Height()

	sub := NewImage16(dx/2, dy/2)

	scaleY := newScale(r.RowY)
	tmp := newImagePredictor(image.Rect(0, 0, int(dx), int(dy)))
	for h := uint16(0); h < dy; h += size {
		for w := uint16(0); w < dx; w += size {
			data, ll, prediction, err := transformLayer(w, h, size, tmp.PredictY, tmp.UpdateY, scaleY, scaleVal)
			if err != nil {
				return nil, nil, 0, errors.WithStack(err)
			}
			bufY = append(bufY, data)
			scaleVal = scaler.CalcScale(data.Len()*8, size*size)

			sub.UpdateY(ll, prediction, w/2, h/2, size/2)
		}
	}

	scaleCb := newScale(r.RowCb)
	for h := uint16(0); h < dy/2; h += size {
		for w := uint16(0); w < dx/2; w += size {
			data, ll, prediction, err := transformLayer(w, h, size, tmp.PredictCb, tmp.UpdateCb, scaleCb, scaleVal)
			if err != nil {
				return nil, nil, 0, errors.WithStack(err)
			}
			bufCb = append(bufCb, data)
			scaleVal = scaler.CalcScale(data.Len()*8, size*size)

			sub.UpdateCb(ll, prediction, w/2, h/2, size/2)
		}
	}

	scaleCr := newScale(r.RowCr)
	for h := uint16(0); h < dy/2; h += size {
		for w := uint16(0); w < dx/2; w += size {
			data, ll, prediction, err := transformLayer(w, h, size, tmp.PredictCr, tmp.UpdateCr, scaleCr, scaleVal)
			if err != nil {
				return nil, nil, 0, errors.WithStack(err)
			}
			bufCr = append(bufCr, data)
			scaleVal = scaler.CalcScale(data.Len()*8, size*size)

			sub.UpdateCr(ll, prediction, w/2, h/2, size/2)
		}
	}

	out := bytes.NewBuffer(nil)
	if err := binary.Write(out, binary.BigEndian, dx); err != nil {
		return nil, nil, 0, errors.WithStack(err)
	}
	if err := binary.Write(out, binary.BigEndian, dy); err != nil {
		return nil, nil, 0, errors.WithStack(err)
	}

	if err := binary.Write(out, binary.BigEndian, uint16(len(bufY))); err != nil {
		return nil, nil, 0, errors.WithStack(err)
	}
	for _, b := range bufY {
		if err := binary.Write(out, binary.BigEndian, uint16(b.Len())); err != nil {
			return nil, nil, 0, errors.WithStack(err)
		}
		if _, err := out.ReadFrom(b); err != nil {
			return nil, nil, 0, errors.WithStack(err)
		}
	}
	if err := binary.Write(out, binary.BigEndian, uint16(len(bufCb))); err != nil {
		return nil, nil, 0, errors.WithStack(err)
	}
	for _, b := range bufCb {
		if err := binary.Write(out, binary.BigEndian, uint16(b.Len())); err != nil {
			return nil, nil, 0, errors.WithStack(err)
		}
		if _, err := out.ReadFrom(b); err != nil {
			return nil, nil, 0, errors.WithStack(err)
		}
	}
	if err := binary.Write(out, binary.BigEndian, uint16(len(bufCr))); err != nil {
		return nil, nil, 0, errors.WithStack(err)
	}
	for _, b := range bufCr {
		if err := binary.Write(out, binary.BigEndian, uint16(b.Len())); err != nil {
			return nil, nil, 0, errors.WithStack(err)
		}
		if _, err := out.ReadFrom(b); err != nil {
			return nil, nil, 0, errors.WithStack(err)
		}
	}
	return out.Bytes(), sub, scaleVal, nil
}

func encodeBase(r *ImageReader, scaler *RateController, scaleVal int, size uint16) ([]byte, error) {
	bufY := make([]*bytes.Buffer, 0)
	bufCb := make([]*bytes.Buffer, 0)
	bufCr := make([]*bytes.Buffer, 0)

	dx, dy := r.Width(), r.Height()

	scaleY := newScale(r.RowY)
	tmp := newImagePredictor(image.Rect(0, 0, int(dx), int(dy)))
	for h := uint16(0); h < dy; h += size {
		for w := uint16(0); w < dx; w += size {
			data, err := transformBase(w, h, size, tmp.PredictY, tmp.UpdateY, scaleY, scaleVal)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			bufY = append(bufY, data)
			scaleVal = scaler.CalcScale(data.Len()*8, size*size)
		}
	}

	scaleCb := newScale(r.RowCb)
	for h := uint16(0); h < dy/2; h += size {
		for w := uint16(0); w < dx/2; w += size {
			data, err := transformBase(w, h, size, tmp.PredictCb, tmp.UpdateCb, scaleCb, scaleVal)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			bufCb = append(bufCb, data)
			scaleVal = scaler.CalcScale(data.Len()*8, size*size)
		}
	}

	scaleCr := newScale(r.RowCr)
	for h := uint16(0); h < dy/2; h += size {
		for w := uint16(0); w < dx/2; w += size {
			data, err := transformBase(w, h, size, tmp.PredictCr, tmp.UpdateCr, scaleCr, scaleVal)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			bufCr = append(bufCr, data)
			scaleVal = scaler.CalcScale(data.Len()*8, size*size)
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

func encode(img *image.YCbCr, maxbitrate int) ([]byte, error) {
	// 全レイヤーの合計ピクセル数を計算して共有 RateController を作成
	// layer2: dx*dy + (dx*dy/2), layer1: dx/2*dy/2 + (dx/2*dy/2)/2, layer0: dx/4*dy/4 + (dx/4*dy/4)/2
	dx, dy := uint16(img.Bounds().Dx()), uint16(img.Bounds().Dy())
	totalPixels := ((int(dx)*int(dy))*3)/2 + ((int(dx/2)*int(dy/2))*3)/2 + ((int(dx/4)*int(dy/4))*3)/2
	scaler := &RateController{
		maxbit:             maxbitrate,
		totalProcessPixels: totalPixels,
		currentBits:        0,
		processedPixels:    0,
		baseShift:          2,
	}

	scaleVal := 1
	r2 := newImageReader(img)
	layer2, sub2, scaleVal, err := encodeLayer(r2, scaler, scaleVal, 32)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	r1 := newImageReader(sub2.ToYCbCr())
	layer1, sub1, scaleVal, err := encodeLayer(r1, scaler, scaleVal, 16)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	r0 := newImageReader(sub1.ToYCbCr())
	layer0, err := encodeBase(r0, scaler, scaleVal, 8)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	out := bytes.NewBuffer(nil)
	if err := binary.Write(out, binary.BigEndian, uint32(len(layer0))); err != nil {
		return nil, errors.WithStack(err)
	}
	if _, err := io.Copy(out, bytes.NewReader(layer0)); err != nil {
		return nil, errors.WithStack(err)
	}

	if err := binary.Write(out, binary.BigEndian, uint32(len(layer1))); err != nil {
		return nil, errors.WithStack(err)
	}
	if _, err := io.Copy(out, bytes.NewReader(layer1)); err != nil {
		return nil, errors.WithStack(err)
	}

	if err := binary.Write(out, binary.BigEndian, uint32(len(layer2))); err != nil {
		return nil, errors.WithStack(err)
	}
	if _, err := io.Copy(out, bytes.NewReader(layer2)); err != nil {
		return nil, errors.WithStack(err)
	}

	return out.Bytes(), nil
}
