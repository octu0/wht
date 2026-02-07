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

func blockEncode(out io.Writer, block []int16, size int) error {
	// int16 -> uint16
	data := unsafe.Slice((*uint16)(unsafe.Pointer(&block[0])), len(block))
	rw := NewRiceWriter[uint16](NewBitWriter(out))
	if err := blockRLEEncode(rw, data); err != nil {
		return errors.WithStack(err)
	}
	if err := rw.Flush(); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func transform(out io.Writer, data [][]int16, size, scale int, isChroma bool) error {
	dwtBlock2Level(data, size)

	// HH1サブバンド(ブロックの右下領域)の解析によるフラット判定
	// Chroma plane の場合は色再現性を優先するため、動的な引き上げを抑制する
	if isChroma == false {
		half := size / 2
		sum := 0
		count := 0
		hhElements := make([]int, 0)
		for y := half; y < size; y++ {
			for x := half; x < size; x++ {
				v := int(data[y][x])
				if v < 0 {
					v = -v
				}
				hhElements = append(hhElements, v)
				sum += v
				count++
			}
		}

		if 0 < count {
			avg := float64(sum) / float64(count)

			// 前方10%と後方10%の平均を比較して分布の安定性を確認
			count10 := count / 10
			if 0 < count10 {
				sumFront, sumBack := 0, 0
				for i := 0; i < count10; i++ {
					sumFront += hhElements[i]
					sumBack += hhElements[len(hhElements)-1-i]
				}
				avgFront := float64(sumFront) / float64(count10)
				avgBack := float64(sumBack) / float64(count10)

				diff := avgFront - avgBack
				if diff < 0 {
					diff = -diff
				}

				// 非常にフラットな場合のみ scale を調整
				if avg < 1.5 && diff < 0.5 {
					scale += 2
				} else if avg < 3.0 && diff < 1.0 {
					scale += 1
				}
			}
		}
	}

	if 15 < scale {
		scale = 15
	}

	if err := binary.Write(out, binary.BigEndian, uint8(scale)); err != nil {
		return errors.WithStack(err)
	}

	quantizeBlock(data, size, scale)

	zigzag := Zigzag(data, size)
	if err := blockEncode(out, zigzag, size); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

type predictFunc func(x, y int, size int) int16
type updatePredictFunc func(x, y int, size int, rows []int16, prediction int16)

func transformHandlerFunc(w, h int, size int, predict predictFunc, updatePredict updatePredictFunc, scale *scale, scaleVal int, isChroma bool) (*bytes.Buffer, error) {
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
	for i := 0; i < size; i += 1 {
		updatePredict(w, h+i, size, planes[i], prediction)
	}
	return data, nil
}

func encode(img *image.YCbCr, maxbitrate int) ([]byte, error) {
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
	mbYSeqRLEBuf := bytes.NewBuffer(nil)
	if err := runlength.NewEncoder(mbYSeqRLEBuf).Encode(mbYSeq); err != nil {
		return nil, errors.WithStack(err)
	}
	mbCbSeqRLEBuf := bytes.NewBuffer(nil)
	if err := runlength.NewEncoder(mbCbSeqRLEBuf).Encode(mbCbSeq); err != nil {
		return nil, errors.WithStack(err)
	}
	mbCrSeqRLEBuf := bytes.NewBuffer(nil)
	if err := runlength.NewEncoder(mbCrSeqRLEBuf).Encode(mbCrSeq); err != nil {
		return nil, errors.WithStack(err)
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

	if err := binary.Write(out, binary.BigEndian, uint32(mbYSeqRLEBuf.Len())); err != nil {
		return nil, errors.WithStack(err)
	}
	if _, err := out.ReadFrom(mbYSeqRLEBuf); err != nil {
		return nil, errors.WithStack(err)
	}
	if err := binary.Write(out, binary.BigEndian, uint32(mbCbSeqRLEBuf.Len())); err != nil {
		return nil, errors.WithStack(err)
	}
	if _, err := out.ReadFrom(mbCbSeqRLEBuf); err != nil {
		return nil, errors.WithStack(err)
	}
	if err := binary.Write(out, binary.BigEndian, uint32(mbCrSeqRLEBuf.Len())); err != nil {
		return nil, errors.WithStack(err)
	}
	if _, err := out.ReadFrom(mbCrSeqRLEBuf); err != nil {
		return nil, errors.WithStack(err)
	}

	return out.Bytes(), nil
}
