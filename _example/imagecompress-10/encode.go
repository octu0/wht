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

func dcEncode(out io.Writer, dc []int16, size int) error {
	rw := NewRiceWriter[uint16](NewBitWriter(out))
	if err := dcRleEncode(rw, dc); err != nil {
		return errors.WithStack(err)
	}
	if err := rw.Flush(); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func acEncode(out io.Writer, zigzag []int8) error {
	// int8 -> uint8
	data := unsafe.Slice((*uint8)(unsafe.Pointer(&zigzag[0])), len(zigzag))
	rw := NewRiceWriter[uint8](NewBitWriter(out))
	if err := rleEncode(rw, data); err != nil {
		return errors.WithStack(err)
	}
	if err := rw.Flush(); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func transform(out io.Writer, dcQT, acQT QuantizationTable, data [][]int16, size, scale int) error {
	if err := binary.Write(out, binary.BigEndian, uint8(scale)); err != nil {
		return errors.WithStack(err)
	}

	dc := make([]int16, size)
	for i := 0; i < size; i += 1 {
		wht.Transform(data[i])
		dc[i] = data[i][0]
	}
	wht.Transform(dc)
	dcQuantize(dc, dcQT)

	if err := dcEncode(out, dc, size); err != nil {
		return errors.WithStack(err)
	}

	qACList := make([][]int8, size)
	for i := 0; i < size; i += 1 {
		qACList[i] = acQuantize(data[i][1:], acQT, scale)
	}

	zigzag := Zigzag(qACList, size, size-1)
	if err := acEncode(out, zigzag); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

type predictFunc func(x, y int, size int) int16
type updatePredictFunc func(x, y int, size int, rows []int16, prediction int16)

func transformHandlerFunc(w, h int, size int, dcQT, acQT QuantizationTable, predict predictFunc, updatePredict updatePredictFunc, scale *scale, scaleVal int) (*bytes.Buffer, error) {
	prediction := predict(w, h, size)
	rows, localScale := scale.Rows(w, h, size, prediction, scaleVal)

	data := bytes.NewBuffer(make([]byte, 0, size*size))
	if err := transform(data, dcQT, acQT, rows, size, localScale); err != nil {
		return nil, errors.WithStack(err)
	}

	// Local Reconstruction
	for i := 0; i < size; i += 1 {
		planes, err := invert(bytes.NewReader(data.Bytes()), dcQT, acQT, size)
		if err != nil {
			return nil, errors.WithStack(err)
		}
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
	scaler := newScaler(maxbitrate, dx, dy)
	tmp := newImagePredictor(img.Bounds())
	for h := 0; h < dy; h += 32 {
		for w := 0; w < dx; w += 32 {
			mb := r.CalcMacroblockY(w, h)
			parts := mbPartitoins[mb]

			for _, p := range parts {
				blockX := w + p.X
				blockY := h + p.Y
				qtPair := qtBySize(p.Size)
				dcQT, lumaQT := qtPair[0], qtPair[1]
				data, err := transformHandlerFunc(blockX, blockY, p.Size, dcQT, lumaQT, tmp.PredictY, tmp.UpdateY, scaleY, scaleVal)
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
				qtPair := qtBySize(p.Size)
				dcQT, chromaQT := qtPair[0], qtPair[2]
				data, err := transformHandlerFunc(blockX, blockY, p.Size, dcQT, chromaQT, tmp.PredictCb, tmp.UpdateCb, scaleCb, scaleVal)
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
				qtPair := qtBySize(p.Size)
				dcQT, chromaQT := qtPair[0], qtPair[2]
				data, err := transformHandlerFunc(blockX, blockY, p.Size, dcQT, chromaQT, tmp.PredictCr, tmp.UpdateCr, scaleCr, scaleVal)
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
