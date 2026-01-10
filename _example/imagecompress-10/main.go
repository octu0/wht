package main

import (
	"bytes"
	"fmt"
	"io"
	"time"

	"github.com/octu0/runlength"
	"github.com/pkg/errors"

	_ "embed"
)

var (
	//go:embed src.png
	srcPng []byte
)

func main() {
	ycbcr, err := pngToYCbCr(srcPng)
	if err != nil {
		panic(fmt.Sprintf("%+v", err))
	}

	srcbit := ycbcr.Bounds().Dx() * ycbcr.Bounds().Dy() * 8
	maxbit := 500 * 1000
	fmt.Printf("src %d bit\n", srcbit)
	fmt.Printf("target %d bit = %3.2f%%\n", maxbit, (float64(maxbit)/float64(srcbit))*100)

	bufY := make([]*bytes.Buffer, 0)
	bufCb := make([]*bytes.Buffer, 0)
	bufCr := make([]*bytes.Buffer, 0)

	minVal, maxVal := int16(32767), int16(-32768)

	type rowFunc func(x, y int, size int, prediction int16) []int16
	rowsAndScale := func(rowN rowFunc, w, h int, size int, prediction int16, scale int) ([][]int16, int) {
		rows := make([][]int16, size)
		// Calculate Range to detect flat blocks
		for i := 0; i < size; i += 1 {
			r := rowN(w, h+i, size, prediction)
			rows[i] = r
			for _, v := range r {
				if v < minVal {
					minVal = v
				}
				if maxVal < v {
					maxVal = v
				}
			}
		}

		localScale := scale
		if (maxVal - minVal) < int16(size) { // Flatness threshold
			localScale = 0
		}

		return rows, localScale
	}

	type predictFunc func(x, y int, size int) int16
	type updatePredictFunc func(x, y int, size int, rows []int16, prediction int16)
	transformHandlerFunc := func(w, h int, size int, dcQT, acQT QuantizationTable, rowN rowFunc, predict predictFunc, updatePredict updatePredictFunc, scale int) (*bytes.Buffer, error) {
		prediction := predict(w, h, size)
		rows, localScale := rowsAndScale(rowN, w, h, size, prediction, scale)

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

	type setRowFunc func(x, y int, size int, plane []int16, prediction int16)
	invertHandlerFunc := func(in io.Reader, w, h int, size int, dcQT, acQT QuantizationTable, predict predictFunc, setRow setRowFunc) error {
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

	mbYSeq := make([]uint8, 0)
	mbCbSeq := make([]uint8, 0)
	mbCrSeq := make([]uint8, 0)
	t := time.Now()
	b := ycbcr.Bounds()
	dx, dy := b.Dx(), b.Dy()

	r := newImageReader(ycbcr)
	scale := 1
	scaler := newScaler(maxbit, dx, dy)
	tmp := newImagePredictor(ycbcr.Bounds())
	for h := 0; h < dy; h += 32 {
		for w := 0; w < dx; w += 32 {
			mb := r.CalcMacroblockY(w, h)
			parts := mbPartitoins[mb]

			for _, p := range parts {
				blockX := w + p.X
				blockY := h + p.Y
				qtPair := qtBySize(p.Size)
				dcQT, lumaQT := qtPair[0], qtPair[1]
				data, err := transformHandlerFunc(blockX, blockY, p.Size, dcQT, lumaQT, r.RowY, tmp.PredictY, tmp.UpdateY, scale)
				if err != nil {
					panic(fmt.Sprintf("%+v", err))
				}
				bufY = append(bufY, data)
				scale = scaler.CalcScale(data.Len()*8, p.Size*p.Size)
			}
			mbYSeq = append(mbYSeq, uint8(mb))
		}
	}
	for h := 0; h < dy/2; h += 16 {
		for w := 0; w < dx/2; w += 16 {
			mb := r.CalcMacroblockCb(w, h)
			parts := mbPartitoins[mb]

			for _, p := range parts {
				blockX := w + p.X
				blockY := h + p.Y
				qtPair := qtBySize(p.Size)
				dcQT, chromaQT := qtPair[0], qtPair[2]
				data, err := transformHandlerFunc(blockX, blockY, p.Size, dcQT, chromaQT, r.RowCb, tmp.PredictCb, tmp.UpdateCb, scale)
				if err != nil {
					panic(fmt.Sprintf("%+v", err))
				}
				bufCb = append(bufCb, data)
				scale = scaler.CalcScale(data.Len()*8, p.Size*p.Size)
			}
			mbCbSeq = append(mbCbSeq, uint8(mb))
		}
	}
	for h := 0; h < dy/2; h += 16 {
		for w := 0; w < dx/2; w += 16 {
			mb := r.CalcMacroblockCr(w, h)
			parts := mbPartitoins[mb]

			for _, p := range parts {
				blockX := w + p.X
				blockY := h + p.Y
				qtPair := qtBySize(p.Size)
				dcQT, chromaQT := qtPair[0], qtPair[2]
				data, err := transformHandlerFunc(blockX, blockY, p.Size, dcQT, chromaQT, r.RowCr, tmp.PredictCr, tmp.UpdateCr, scale)
				if err != nil {
					panic(fmt.Sprintf("%+v", err))
				}
				bufCr = append(bufCr, data)
				scale = scaler.CalcScale(data.Len()*8, p.Size*p.Size)
			}
			mbCrSeq = append(mbCrSeq, uint8(mb))
		}
	}

	original := len(ycbcr.Y) + len(ycbcr.Cb) + len(ycbcr.Cr)
	compressedSize := 0
	for _, b := range bufY {
		compressedSize += b.Len()
	}
	for _, b := range bufCb {
		compressedSize += b.Len()
	}
	for _, b := range bufCr {
		compressedSize += b.Len()
	}

	mbYSeqRLEBuf := bytes.NewBuffer(nil)
	if err := runlength.NewEncoder(mbYSeqRLEBuf).Encode(mbYSeq); err != nil {
		panic(fmt.Sprintf("%+v", err))
	}
	mbCbSeqRLEBuf := bytes.NewBuffer(nil)
	if err := runlength.NewEncoder(mbCbSeqRLEBuf).Encode(mbCbSeq); err != nil {
		panic(fmt.Sprintf("%+v", err))
	}
	mbCrSeqRLEBuf := bytes.NewBuffer(nil)
	if err := runlength.NewEncoder(mbCrSeqRLEBuf).Encode(mbCrSeq); err != nil {
		panic(fmt.Sprintf("%+v", err))
	}
	compressedSize += mbYSeqRLEBuf.Len()
	compressedSize += mbCbSeqRLEBuf.Len()
	compressedSize += mbCrSeqRLEBuf.Len()

	fmt.Printf(
		"elapse=%s %3.2fKB -> %3.2fKB compressed %3.2f%%\n",
		time.Since(t),
		float64(original)/1024.0,
		float64(compressedSize)/1024.0,
		(float64(compressedSize)/float64(original))*100,
	)

	ip := newImagePredictor(ycbcr.Bounds())
	for h := 0; h < dy; h += 32 {
		for w := 0; w < dx; w += 32 {
			mb := macroblock(mbYSeq[0])
			mbYSeq = mbYSeq[1:]
			parts := mbPartitoins[mb]

			for _, p := range parts {
				blockX := w + p.X
				blockY := h + p.Y
				data := bufY[0]
				bufY = bufY[1:]
				in := bytes.NewReader(data.Bytes())

				qtPair := qtBySize(p.Size)
				dcQT, lumaQT := qtPair[0], qtPair[1]
				if err := invertHandlerFunc(in, blockX, blockY, p.Size, dcQT, lumaQT, ip.PredictY, ip.UpdateY); err != nil {
					panic(fmt.Sprintf("%+v", err))
				}
			}
		}
	}
	for h := 0; h < dy/2; h += 16 {
		for w := 0; w < dx/2; w += 16 {
			mb := macroblock(mbCbSeq[0])
			mbCbSeq = mbCbSeq[1:]
			parts := mbPartitoins[mb]

			for _, p := range parts {
				blockX := w + p.X
				blockY := h + p.Y
				data := bufCb[0]
				bufCb = bufCb[1:]
				in := bytes.NewReader(data.Bytes())

				qtPair := qtBySize(p.Size)
				dcQT, chromaQT := qtPair[0], qtPair[2]
				if err := invertHandlerFunc(in, blockX, blockY, p.Size, dcQT, chromaQT, ip.PredictCb, ip.UpdateCb); err != nil {
					panic(fmt.Sprintf("%+v", err))
				}
			}
		}
	}
	for h := 0; h < dy/2; h += 16 {
		for w := 0; w < dx/2; w += 16 {
			mb := macroblock(mbCrSeq[0])
			mbCrSeq = mbCrSeq[1:]
			parts := mbPartitoins[mb]

			for _, p := range parts {
				blockX := w + p.X
				blockY := h + p.Y
				data := bufCr[0]
				bufCr = bufCr[1:]
				in := bytes.NewReader(data.Bytes())

				qtPair := qtBySize(p.Size)
				dcQT, chromaQT := qtPair[0], qtPair[2]
				if err := invertHandlerFunc(in, blockX, blockY, p.Size, dcQT, chromaQT, ip.PredictCr, ip.UpdateCr); err != nil {
					panic(fmt.Sprintf("%+v", err))
				}
			}
		}
	}

	ip.Deblocking()

	if err := saveImage(ycbcr, "out_origin.png"); err != nil {
		panic(fmt.Sprintf("%+v", err))
	}
	if err := saveImage(ip.img, "out_new.png"); err != nil {
		panic(fmt.Sprintf("%+v", err))
	}
}
