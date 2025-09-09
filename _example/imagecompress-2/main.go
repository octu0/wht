package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"time"
	"unsafe"

	"github.com/octu0/runlength"
	"github.com/octu0/wht"
	"github.com/pkg/errors"

	_ "embed"
)

var (
	//go:embed src.png
	srcPng []byte
)

var (
	lumaQuantizationTable = [16]int16{
		8, 8, 8, 8,
		16, 16, 16, 16,
		16, 24, 32, 48,
		64, 80, 96, 96,
	}
	chromaQuantizationTable = [16]int16{
		64, 64, 80, 90,
		100, 112, 128, 160,
		176, 192, 208, 208,
		220, 220, 240, 240,
	}
)

type (
	dictKey [4]uint8
	dict    map[dictKey]uint16
)

var (
	dictIndex = uint16(0)
)

func (d dict) Has(k0, k1, k2, k3 uint8) bool {
	k := [4]uint8{k0, k1, k2, k3}
	_, ok := d[k]
	return ok
}

func (d dict) Add(k0, k1, k2, k3 uint8) uint16 {
	k := [4]uint8{k0, k1, k2, k3}
	idx, ok := d[k]
	if ok {
		return idx
	}
	next := dictIndex + 1
	d[k] = next
	dictIndex = next
	return next
}

func (d dict) Search(idx uint16) ([4]uint8, bool) {
	for k, v := range d {
		if v == idx {
			return k, true
		}
	}
	return [4]uint8{}, false
}

func (d dict) Dump(out io.Writer) error {
	size := len(d)
	if err := binary.Write(out, binary.BigEndian, uint16(size)); err != nil {
		return errors.WithStack(err)
	}
	for k, v := range d {
		if err := binary.Write(out, binary.BigEndian, k[0]); err != nil {
			return errors.WithStack(err)
		}
		if err := binary.Write(out, binary.BigEndian, k[1]); err != nil {
			return errors.WithStack(err)
		}
		if err := binary.Write(out, binary.BigEndian, k[2]); err != nil {
			return errors.WithStack(err)
		}
		if err := binary.Write(out, binary.BigEndian, k[3]); err != nil {
			return errors.WithStack(err)
		}
		if err := binary.Write(out, binary.BigEndian, v); err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

func (d dict) ReadFrom(in io.Reader) error {
	size := uint16(0)
	if err := binary.Read(in, binary.BigEndian, &size); err != nil {
		return errors.WithStack(err)
	}
	for i := uint16(0); i < size; i += 1 {
		k0, k1, k2, k3 := uint8(0), uint8(0), uint8(0), uint8(0)
		dictIndex := uint16(0)
		if err := binary.Read(in, binary.BigEndian, &k0); err != nil {
			return errors.WithStack(err)
		}
		if err := binary.Read(in, binary.BigEndian, &k1); err != nil {
			return errors.WithStack(err)
		}
		if err := binary.Read(in, binary.BigEndian, &k2); err != nil {
			return errors.WithStack(err)
		}
		if err := binary.Read(in, binary.BigEndian, &k3); err != nil {
			return errors.WithStack(err)
		}
		if err := binary.Read(in, binary.BigEndian, &dictIndex); err != nil {
			return errors.WithStack(err)
		}
		key := [4]uint8{k0, k1, k2, k3}
		d[key] = dictIndex
	}
	return nil
}

func main() {
	ycbcr, err := pngToYCbCr(srcPng)
	if err != nil {
		panic(fmt.Sprintf("%+v", err))
	}
	row16 := func(w, h int) (y, cb, cr []int16) {
		y = make([]int16, 16)
		cb = make([]int16, 16)
		cr = make([]int16, 16)
		// - 128 = level shift
		for i := 0; i < 16; i += 1 {
			c := ycbcr.YCbCrAt(w+i, h)
			y[i] = int16(c.Y) - 128
			cb[i] = int16(c.Cb) - 128
			cr[i] = int16(c.Cr) - 128
		}
		return y, cb, cr
	}

	quantize := func(data []int16, quantizeTable [16]int16) []int8 {
		result := make([]int8, len(data))
		for i, q := range quantizeTable {
			v := data[i] / q
			switch {
			case 127 < v:
				v = 127
			case v < -128:
				v = -128
			}
			result[i] = int8(v)
		}
		return result
	}
	dequantize := func(data []int8, quantizeTable [16]int16) []int16 {
		result := make([]int16, len(data))
		for i, q := range quantizeTable {
			result[i] = int16(data[i]) * q
		}
		return result
	}
	clampU8 := func(v int16) uint8 {
		if v < 0 {
			return 0
		}
		if v > 255 {
			return 255
		}
		return uint8(v)
	}

	d := make(dict)
	encBuf := bytes.NewBuffer(nil)
	encode := func(out io.Writer, dc []int16, ac []int8) error {
		for i := 0; i < 16; i += 1 {
			if err := binary.Write(out, binary.BigEndian, dc[i]); err != nil {
				return errors.WithStack(err)
			}
		}
		encBuf.Reset()

		// int8 -> uint8
		data := unsafe.Slice((*uint8)(unsafe.Pointer(&ac[0])), len(ac))
		if err := runlength.NewEncoder(encBuf).Encode(data); err != nil {
			return errors.WithStack(err)
		}
		encoded := encBuf.Bytes()
		if len(encoded) < 16 { // no compress
			if _, err := out.Write([]byte{0xf0}); err != nil {
				return errors.WithStack(err)
			}
			if _, err := encBuf.WriteTo(out); err != nil {
				return errors.WithStack(err)
			}
			return nil
		}

		// compress
		if _, err := out.Write([]byte{0xf1}); err != nil {
			return errors.WithStack(err)
		}
		size := uint16(len(encoded) + 3/4)
		if err := binary.Write(out, binary.BigEndian, size); err != nil {
			return errors.WithStack(err)
		}
		for i := 0; i < len(encoded); i += 4 {
			k0 := encoded[i]
			k1 := encoded[i+1]
			k2 := uint8(0)
			k3 := uint8(0)
			if i+2 < len(encoded) {
				k2 = encoded[i+2]
			}
			if i+3 < len(encoded) {
				k3 = encoded[i+3]
			}
			dictIndex := d.Add(k0, k1, k2, k3)
			if err := binary.Write(out, binary.BigEndian, dictIndex); err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	}
	decBuf := bytes.NewBuffer(nil)
	decode := func(in io.Reader) ([]int16, []int8, error) {
		dc := make([]int16, 16)
		for i := 0; i < 16; i += 1 {
			i16 := int16(0)
			if err := binary.Read(in, binary.BigEndian, &i16); err != nil {
				return nil, nil, errors.WithStack(err)
			}
			dc[i] = i16
		}
		decBuf.Reset()

		compressMode := uint8(0)
		if err := binary.Read(in, binary.BigEndian, &compressMode); err != nil {
			return nil, nil, errors.WithStack(err)
		}
		if compressMode == 0xf0 {
			b, err := runlength.NewDecoder().Decode(in)
			if err != nil {
				return nil, nil, errors.WithStack(err)
			}
			// uint8 -> int8
			ac := unsafe.Slice((*int8)(unsafe.Pointer(&b[0])), len(b))
			return dc, ac, nil
		}

		size := uint16(0)
		if err := binary.Read(in, binary.BigEndian, &size); err != nil {
			return nil, nil, errors.WithStack(err)
		}
		for i := uint16(0); i < size; i += 1 {
			dictIndex := uint16(0)
			if err := binary.Read(in, binary.BigEndian, &dictIndex); err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				return nil, nil, errors.WithStack(err)
			}
			k, ok := d.Search(dictIndex)
			if ok != true {
				panic(fmt.Sprintf("idx: %d not ound", dictIndex))
			}
			if _, err := decBuf.Write([]byte{k[0], k[1], k[2], k[3]}); err != nil {
				return nil, nil, errors.WithStack(err)
			}
		}

		b, err := runlength.NewDecoder().Decode(bytes.NewReader(decBuf.Bytes()))
		if err != nil {
			return nil, nil, errors.WithStack(err)
		}
		// uint8 -> int8
		ac := unsafe.Slice((*int8)(unsafe.Pointer(&b[0])), len(b))
		return dc, ac, nil
	}

	transform := func(out io.Writer, qt [16]int16, d0, d1, d2, d3, d4, d5, d6, d7, d8, d9, d10, d11, d12, d13, d14, d15 []int16) error {
		wht.Transform(d0)
		wht.Transform(d1)
		wht.Transform(d2)
		wht.Transform(d3)
		wht.Transform(d4)
		wht.Transform(d5)
		wht.Transform(d6)
		wht.Transform(d7)
		wht.Transform(d8)
		wht.Transform(d9)
		wht.Transform(d10)
		wht.Transform(d11)
		wht.Transform(d12)
		wht.Transform(d13)
		wht.Transform(d14)
		wht.Transform(d15)

		dc := []int16{
			d0[0],
			d1[0],
			d2[0],
			d3[0],
			d4[0],
			d5[0],
			d6[0],
			d7[0],
			d8[0],
			d9[0],
			d10[0],
			d11[0],
			d12[0],
			d13[0],
			d14[0],
			d15[0],
		}
		wht.Transform(dc)
		//quantize(dc, qt)
		qAC0 := quantize(d0, qt)
		qAC1 := quantize(d1, qt)
		qAC2 := quantize(d2, qt)
		qAC3 := quantize(d3, qt)
		qAC4 := quantize(d4, qt)
		qAC5 := quantize(d5, qt)
		qAC6 := quantize(d6, qt)
		qAC7 := quantize(d7, qt)
		qAC8 := quantize(d8, qt)
		qAC9 := quantize(d9, qt)
		qAC10 := quantize(d10, qt)
		qAC11 := quantize(d11, qt)
		qAC12 := quantize(d12, qt)
		qAC13 := quantize(d13, qt)
		qAC14 := quantize(d14, qt)
		qAC15 := quantize(d15, qt)

		zigzag := wht.Zigzag([][]int8{
			qAC0,
			qAC1,
			qAC2,
			qAC3,
			qAC4,
			qAC5,
			qAC6,
			qAC7,
			qAC8,
			qAC9,
			qAC10,
			qAC11,
			qAC12,
			qAC13,
			qAC14,
			qAC15,
		})
		if err := encode(out, dc, zigzag); err != nil {
			return errors.WithStack(err)
		}
		return nil
	}

	invert := func(data []uint8, qt [16]int16) ([][]int16, error) {
		dc, zigzag, err := decode(bytes.NewReader(data))
		if err != nil {
			return nil, errors.WithStack(err)
		}
		acPlanes := wht.Unzigzag(zigzag, 16)
		//dequantize(dc, qt)
		wht.Invert(dc)

		ac0 := dequantize(acPlanes[0], qt)
		ac1 := dequantize(acPlanes[1], qt)
		ac2 := dequantize(acPlanes[2], qt)
		ac3 := dequantize(acPlanes[3], qt)
		ac4 := dequantize(acPlanes[4], qt)
		ac5 := dequantize(acPlanes[5], qt)
		ac6 := dequantize(acPlanes[6], qt)
		ac7 := dequantize(acPlanes[7], qt)
		ac8 := dequantize(acPlanes[8], qt)
		ac9 := dequantize(acPlanes[9], qt)
		ac10 := dequantize(acPlanes[10], qt)
		ac11 := dequantize(acPlanes[11], qt)
		ac12 := dequantize(acPlanes[12], qt)
		ac13 := dequantize(acPlanes[13], qt)
		ac14 := dequantize(acPlanes[14], qt)
		ac15 := dequantize(acPlanes[15], qt)

		ac0[0] = dc[0]
		ac1[0] = dc[1]
		ac2[0] = dc[2]
		ac3[0] = dc[3]
		ac4[0] = dc[4]
		ac5[0] = dc[5]
		ac6[0] = dc[6]
		ac7[0] = dc[7]
		ac8[0] = dc[8]
		ac9[0] = dc[9]
		ac10[0] = dc[10]
		ac11[0] = dc[11]
		ac12[0] = dc[12]
		ac13[0] = dc[13]
		ac14[0] = dc[14]
		ac15[0] = dc[15]

		wht.Invert(ac0)
		wht.Invert(ac1)
		wht.Invert(ac2)
		wht.Invert(ac3)
		wht.Invert(ac4)
		wht.Invert(ac5)
		wht.Invert(ac6)
		wht.Invert(ac7)
		wht.Invert(ac8)
		wht.Invert(ac9)
		wht.Invert(ac10)
		wht.Invert(ac11)
		wht.Invert(ac12)
		wht.Invert(ac13)
		wht.Invert(ac14)
		wht.Invert(ac15)

		return [][]int16{
			ac0,
			ac1,
			ac2,
			ac3,
			ac4,
			ac5,
			ac6,
			ac7,
			ac8,
			ac9,
			ac10,
			ac11,
			ac12,
			ac13,
			ac14,
			ac15,
		}, nil
	}

	bufY := make([]*bytes.Buffer, 0)
	bufCb := make([]*bytes.Buffer, 0)
	bufCr := make([]*bytes.Buffer, 0)

	t := time.Now()
	compressedSize := 0
	b := ycbcr.Bounds()
	dx, dy := b.Dx(), b.Dy()
	for h := 0; h < dy; h += 16 {
		for w := 0; w < dx; w += 16 {
			rowY0, rowCb0, rowCr0 := row16(w, h+0)
			rowY1, rowCb1, rowCr1 := row16(w, h+1)
			rowY2, rowCb2, rowCr2 := row16(w, h+2)
			rowY3, rowCb3, rowCr3 := row16(w, h+3)
			rowY4, rowCb4, rowCr4 := row16(w, h+4)
			rowY5, rowCb5, rowCr5 := row16(w, h+5)
			rowY6, rowCb6, rowCr6 := row16(w, h+6)
			rowY7, rowCb7, rowCr7 := row16(w, h+7)
			rowY8, rowCb8, rowCr8 := row16(w, h+8)
			rowY9, rowCb9, rowCr9 := row16(w, h+9)
			rowY10, rowCb10, rowCr10 := row16(w, h+10)
			rowY11, rowCb11, rowCr11 := row16(w, h+11)
			rowY12, rowCb12, rowCr12 := row16(w, h+12)
			rowY13, rowCb13, rowCr13 := row16(w, h+13)
			rowY14, rowCb14, rowCr14 := row16(w, h+14)
			rowY15, rowCb15, rowCr15 := row16(w, h+15)

			dataY := bytes.NewBuffer(make([]byte, 0, 16*16))
			if err := transform(
				dataY,
				lumaQuantizationTable,
				rowY0, rowY1, rowY2, rowY3,
				rowY4, rowY5, rowY6, rowY7,
				rowY8, rowY9, rowY10, rowY11,
				rowY12, rowY13, rowY14, rowY15,
			); err != nil {
				panic(err)
			}
			dataCb := bytes.NewBuffer(make([]byte, 0, 16*16))
			if err := transform(
				dataCb,
				chromaQuantizationTable,
				rowCb0, rowCb1, rowCb2, rowCb3,
				rowCb4, rowCb5, rowCb6, rowCb7,
				rowCb8, rowCb9, rowCb10, rowCb11,
				rowCb12, rowCb13, rowCb14, rowCb15,
			); err != nil {
				panic(err)
			}
			dataCr := bytes.NewBuffer(make([]byte, 0, 16*16))
			if err := transform(
				dataCr,
				chromaQuantizationTable,
				rowCr0, rowCr1, rowCr2, rowCr3,
				rowCr4, rowCr5, rowCr6, rowCr7,
				rowCr8, rowCr9, rowCr10, rowCr11,
				rowCr12, rowCr13, rowCr14, rowCr15,
			); err != nil {
				panic(err)
			}
			compressedSize += dataY.Len() + dataCb.Len() + dataCr.Len()

			bufY = append(bufY, dataY)
			bufCb = append(bufCb, dataCb)
			bufCr = append(bufCr, dataCr)
		}
	}
	original := len(ycbcr.Y) + len(ycbcr.Cb) + len(ycbcr.Cr)
	dump := bytes.NewBuffer(nil)
	d.Dump(dump)
	compressedSize += dump.Len()
	fmt.Printf("elapse=%s compressed %3.2f%%\n", time.Since(t), (float64(compressedSize)/float64(original))*100)

	newImg := image.NewYCbCr(b, image.YCbCrSubsampleRatio444)
	setRow16 := func(x, y int, yPlane, cbPlane, crPlane []int16) {
		defer func() {
			if r := recover(); r != nil {
			}
		}()
		for i := 0; i < 16; i += 1 {
			newImg.Y[newImg.YOffset(x+i, y)] = clampU8(yPlane[i] + 128)
			newImg.Cb[newImg.COffset(x+i, y)] = clampU8(cbPlane[i] + 128)
			newImg.Cr[newImg.COffset(x+i, y)] = clampU8(crPlane[i] + 128)
		}
	}
	for h := 0; h < dy; h += 16 {
		for w := 0; w < dx; w += 16 {
			dataY := bufY[0]
			dataCb := bufCb[0]
			dataCr := bufCr[0]
			bufY = bufY[1:]
			bufCb = bufCb[1:]
			bufCr = bufCr[1:]

			yPlanes, err := invert(dataY.Bytes(), lumaQuantizationTable)
			if err != nil {
				panic(fmt.Sprintf("%+v", err))
			}
			rowY0, rowY1, rowY2, rowY3,
				rowY4, rowY5, rowY6, rowY7,
				rowY8, rowY9, rowY10, rowY11,
				rowY12, rowY13, rowY14, rowY15 := yPlanes[0], yPlanes[1], yPlanes[2], yPlanes[3], yPlanes[4], yPlanes[5], yPlanes[6], yPlanes[7], yPlanes[8], yPlanes[9], yPlanes[10], yPlanes[11], yPlanes[12], yPlanes[13], yPlanes[14], yPlanes[15]

			cbPlanes, err := invert(dataCb.Bytes(), chromaQuantizationTable)
			if err != nil {
				panic(fmt.Sprintf("%+v", err))
			}
			rowCb0, rowCb1, rowCb2, rowCb3,
				rowCb4, rowCb5, rowCb6, rowCb7,
				rowCb8, rowCb9, rowCb10, rowCb11,
				rowCb12, rowCb13, rowCb14, rowCb15 := cbPlanes[0], cbPlanes[1], cbPlanes[2], cbPlanes[3], cbPlanes[4], cbPlanes[5], cbPlanes[6], cbPlanes[7], cbPlanes[8], cbPlanes[9], cbPlanes[10], cbPlanes[11], cbPlanes[12], cbPlanes[13], cbPlanes[14], cbPlanes[15]

			crPlanes, err := invert(dataCr.Bytes(), chromaQuantizationTable)
			if err != nil {
				panic(fmt.Sprintf("%+v", err))
			}
			rowCr0, rowCr1, rowCr2, rowCr3,
				rowCr4, rowCr5, rowCr6, rowCr7,
				rowCr8, rowCr9, rowCr10, rowCr11,
				rowCr12, rowCr13, rowCr14, rowCr15 := crPlanes[0], crPlanes[1], crPlanes[2], crPlanes[3], crPlanes[4], crPlanes[5], crPlanes[6], crPlanes[7], crPlanes[8], crPlanes[9], crPlanes[10], crPlanes[11], crPlanes[12], crPlanes[13], crPlanes[14], crPlanes[15]

			setRow16(w, h+0, rowY0, rowCb0, rowCr0)
			setRow16(w, h+1, rowY1, rowCb1, rowCr1)
			setRow16(w, h+2, rowY2, rowCb2, rowCr2)
			setRow16(w, h+3, rowY3, rowCb3, rowCr3)
			setRow16(w, h+4, rowY4, rowCb4, rowCr4)
			setRow16(w, h+5, rowY5, rowCb5, rowCr5)
			setRow16(w, h+6, rowY6, rowCb6, rowCr6)
			setRow16(w, h+7, rowY7, rowCb7, rowCr7)
			setRow16(w, h+8, rowY8, rowCb8, rowCr8)
			setRow16(w, h+9, rowY9, rowCb9, rowCr9)
			setRow16(w, h+10, rowY10, rowCb10, rowCr10)
			setRow16(w, h+11, rowY11, rowCb11, rowCr11)
			setRow16(w, h+12, rowY12, rowCb12, rowCr12)
			setRow16(w, h+13, rowY13, rowCb13, rowCr13)
			setRow16(w, h+14, rowY14, rowCb14, rowCr14)
			setRow16(w, h+15, rowY15, rowCb15, rowCr15)
		}
	}

	// avg deblocking
	for h := 0; h < dy; h += 1 {
		for w := 15; w < dx-1; w += 15 {
			p0 := int16(newImg.Y[newImg.YOffset(w-2, h)]) // x=13
			p1 := int16(newImg.Y[newImg.YOffset(w-1, h)]) // x=14
			p2 := int16(newImg.Y[newImg.YOffset(w, h)])   // x=15
			p3 := int16(newImg.Y[newImg.YOffset(w+1, h)]) // x=16
			p4 := int16(newImg.Y[newImg.YOffset(w+2, h)]) // x=17
			avg := (p0 + p1 + p2 + p3 + p4) / 5
			newImg.Y[newImg.YOffset(w, h)] = uint8((p2*2 + avg) / 3)
			newImg.Y[newImg.YOffset(w+1, h)] = uint8((p3*2 + avg) / 3)
		}
	}

	for h := 0; h < dy; h += 1 {
		for w := 15; w < dx-1; w += 8 {
			p1 := int16(newImg.Y[newImg.YOffset(w-1, h)]) // x=14
			p2 := int16(newImg.Y[newImg.YOffset(w, h)])   // x=15
			p3 := int16(newImg.Y[newImg.YOffset(w+1, h)]) // x=16
			avg := (p1 + p2 + p3) / 3
			newImg.Y[newImg.YOffset(w, h)] = uint8((p2*2 + avg) / 3)
			newImg.Y[newImg.YOffset(w+1, h)] = uint8((p3*2 + avg) / 3)
		}
	}

	if err := saveImage(ycbcr, "out_origin.png"); err != nil {
		panic(fmt.Sprintf("%+v", err))
	}
	if err := saveImage(newImg, "out_new.png"); err != nil {
		panic(fmt.Sprintf("%+v", err))
	}
}

func pngToYCbCr(data []byte) (*image.YCbCr, error) {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, errors.WithStack(err)
	}

	ycbcr := image.NewYCbCr(img.Bounds(), image.YCbCrSubsampleRatio444)
	if err := convertYCbCr(ycbcr, img); err != nil {
		return nil, errors.WithStack(err)
	}
	return ycbcr, nil
}

func saveImage(img image.Image, name string) error {
	out, err := os.Create(name)
	if err != nil {
		return errors.WithStack(err)
	}
	defer out.Close()

	if err := png.Encode(out, img); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func convertYCbCr(dst *image.YCbCr, src image.Image) error {
	rect := src.Bounds()
	width, height := rect.Dx(), rect.Dy()

	for w := 0; w < width; w += 1 {
		for h := 0; h < height; h += 1 {
			c := src.At(w, h)
			r, g, b, _ := c.RGBA()
			y, u, v := color.RGBToYCbCr(uint8(r>>8), uint8(g>>8), uint8(b>>8))
			dst.Y[dst.YOffset(w, h)] = y
			dst.Cb[dst.COffset(w, h)] = u
			dst.Cr[dst.COffset(w, h)] = v
		}
	}
	return nil
}
