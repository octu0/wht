package main

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"iter"
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

type compressMode uint8

const (
	noCompress  compressMode = 0xf0
	rleCompress compressMode = 0xf1
)

func (c compressMode) Bytes() []byte {
	return []byte{byte(c)}
}

type macroblock uint8

const (
	mb8_4x8_4  macroblock = 1
	mb16_2x8_4 macroblock = 2
	mb16_2x2   macroblock = 3
	mb32x32    macroblock = 4
)

type macroblockPartition struct {
	X, Y, Size int
}

var (
	mbPartitoins = map[macroblock][]macroblockPartition{
		mb8_4x8_4: {
			{0, 0, 8}, {8, 0, 8}, {16, 0, 8}, {24, 0, 8},
			{0, 8, 8}, {8, 8, 8}, {16, 8, 8}, {24, 8, 8},
			{0, 16, 8}, {8, 16, 8}, {16, 16, 8}, {24, 16, 8},
			{0, 24, 8}, {8, 24, 8}, {16, 24, 8}, {24, 24, 8},
		},
		mb16_2x8_4: {
			{0, 0, 16},
			{16, 0, 8}, {24, 0, 8}, {16, 8, 8}, {24, 8, 8},
			{0, 16, 8}, {8, 16, 8}, {0, 24, 8}, {8, 24, 8},
			{16, 16, 16},
		},
		mb16_2x2: {
			{0, 0, 16},
			{16, 0, 16},
			{0, 16, 16},
			{16, 16, 16},
		},
		mb32x32: {
			{0, 0, 32},
		},
	}
)

var (
	lumaQuantizationTable8 = []int16{
		8, 8, 8, 8,
		16, 16, 24, 32,
	}
	lumaQuantizationTable16 = []int16{
		8, 8, 8, 8,
		16, 16, 16, 16,
		16, 24, 32, 48,
		64, 80, 96, 96,
	}
	lumaQuantizationTable32 = []int16{
		8, 8, 8, 8,
		8, 8, 16, 16,
		16, 16, 16, 16,
		16, 16, 24, 24,
		24, 24, 32, 32,
		32, 32, 32, 32,
		48, 48, 48, 64,
		64, 80, 96, 96,
	}
	chromaQuantizationTable8 = []int16{
		64, 64, 64, 64,
		100, 128, 192, 208,
	}
	chromaQuantizationTable16 = []int16{
		64, 64, 80, 90,
		100, 112, 128, 160,
		176, 192, 208, 208,
		220, 220, 240, 240,
	}
	chromaQuantizationTable32 = []int16{
		64, 64, 64, 64,
		64, 64, 64, 90,
		90, 90, 90, 90,
		100, 100, 110, 110,
		128, 128, 144, 160,
		176, 192, 208, 208,
		220, 220, 240, 240,
		240, 240, 240, 240,
	}
)

type QuantizationTable []int16

func (qt QuantizationTable) Get() []int16 {
	return []int16(qt)
}

var (
	lumaQT8    = QuantizationTable(lumaQuantizationTable8)
	lumaQT16   = QuantizationTable(lumaQuantizationTable16)
	lumaQT32   = QuantizationTable(lumaQuantizationTable32)
	chromaQT8  = QuantizationTable(chromaQuantizationTable8)
	chromaQT16 = QuantizationTable(chromaQuantizationTable16)
	chromaQT32 = QuantizationTable(chromaQuantizationTable32)
)

func qtBySize(size int) [2]QuantizationTable {
	switch macroblock(size) {
	case 8:
		return [2]QuantizationTable{lumaQT8, chromaQT8}
	case 16:
		return [2]QuantizationTable{lumaQT16, chromaQT16}
	case 32:
		return [2]QuantizationTable{lumaQT32, chromaQT32}
	}
	panic(fmt.Sprintf("size:%d not found", size))
}

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

type dpcmEncode struct {
	buf  *bytes.Buffer
	huff *flate.Writer
}

func (e *dpcmEncode) Write(dc []int16) error {
	out := bytes.NewBuffer(nil)
	for v := range e.encode(dc) {
		if err := binary.Write(out, binary.BigEndian, v); err != nil {
			return errors.WithStack(err)
		}
	}
	if _, err := e.huff.Write(out.Bytes()); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func (e *dpcmEncode) encode(dc []int16) iter.Seq[int16] {
	return func(yield func(int16) bool) {
		if yield(dc[0]) != true {
			return
		}
		v := dc[0]
		for i := 1; i < len(dc); i += 1 {
			v = dc[i] - dc[i-1]
			if yield(v) != true {
				return
			}
		}
	}
}

func (e *dpcmEncode) Close() error {
	if err := e.huff.Close(); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func (e *dpcmEncode) Bytes() []byte {
	return e.buf.Bytes()
}

func (e *dpcmEncode) Len() int {
	return e.buf.Len()
}

func createDPCMEncode() (*dpcmEncode, error) {
	buf := bytes.NewBuffer(nil)
	huff, err := flate.NewWriter(buf, flate.HuffmanOnly)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &dpcmEncode{buf, huff}, nil
}

type dpcmDecode struct {
	huff io.ReadCloser
}

func (d *dpcmDecode) ReadSize(size int) ([]int16, error) {
	decoded := make([]int16, 0, size)
	for v, err := range d.read(size) {
		if err != nil {
			return nil, errors.WithStack(err)
		}
		decoded = append(decoded, v)
	}

	result := make([]int16, size)
	result[0] = decoded[0]
	for i := 1; i < size; i += 1 {
		result[i] = result[i-1] + decoded[i]
	}
	return result, nil
}

func (d *dpcmDecode) read(size int) iter.Seq2[int16, error] {
	return func(yield func(int16, error) bool) {
		for i := 0; i < size; i += 1 {
			v := int16(0)
			if err := binary.Read(d.huff, binary.BigEndian, &v); err != nil {
				yield(0, errors.WithStack(err))
				return
			}
			if yield(v, nil) != true {
				return
			}
		}
	}
}

func (d *dpcmDecode) Close() error {
	if err := d.huff.Close(); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func createDPCMDecode(in io.Reader) (*dpcmDecode, error) {
	huff := flate.NewReader(in)
	return &dpcmDecode{huff}, nil
}

func main() {
	ycbcr, err := pngToYCbCr(srcPng)
	if err != nil {
		panic(fmt.Sprintf("%+v", err))
	}
	headtailY := func(w, h int, n int) (uint8, uint8) {
		i := (h * ycbcr.YStride) + w
		if len(ycbcr.Y) < i {
			return 0, 0
		}
		if len(ycbcr.Y) < i+n {
			return ycbcr.Y[i], 0
		}
		return ycbcr.Y[i], ycbcr.Y[i+n]
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
	rowN := func(w, h int, size int) (y, cb, cr []int16) {
		y = make([]int16, size)
		cb = make([]int16, size)
		cr = make([]int16, size)
		// - 128 = level shift
		for i := 0; i < size; i += 1 {
			c := ycbcr.YCbCrAt(w+i, h)
			y[i] = int16(c.Y) - 128
			cb[i] = int16(c.Cb) - 128
			cr[i] = int16(c.Cr) - 128
		}
		return y, cb, cr
	}
	newImg := image.NewYCbCr(ycbcr.Bounds(), image.YCbCrSubsampleRatio444)
	setRowN := func(x, y int, size int, yPlane, cbPlane, crPlane []int16) {
		defer func() {
			if r := recover(); r != nil {
			}
		}()
		for i := 0; i < size; i += 1 {
			newImg.Y[newImg.YOffset(x+i, y)] = clampU8(yPlane[i] + 128)
			newImg.Cb[newImg.COffset(x+i, y)] = clampU8(cbPlane[i] + 128)
			newImg.Cr[newImg.COffset(x+i, y)] = clampU8(crPlane[i] + 128)
		}
	}

	similarblock := func(w, h int, size int) bool {
		h0, t0 := headtailY(w, h, size)
		h1, t1 := headtailY(w, h+(size-1), size)

		mi := min(min(h0, h1), min(t0, t1))
		ma := max(max(h0, h1), max(t0, t1))
		if (ma - mi) < 32 { // 32/255 = 0.1254
			return true
		}
		return false
	}
	similarblockNeighbor := func(w, h int, size, next int) bool {
		h0, t0 := headtailY(w, h, size)
		h1, t1 := headtailY(w, h+(size-1), size)
		h2, t2 := headtailY(w, h+(size+next-1), size)

		mi := min(min(min(h0, h1), min(t0, t1)), min(h2, t2))
		ma := max(max(max(h0, h1), max(t0, t1)), max(h2, t2))
		if (ma - mi) < 16 { // 16/255 = 0.0627
			return true
		}
		return false
	}
	calcMacroblock := func(w, h int) macroblock {
		if similarblock(w, h, 16) {
			if similarblockNeighbor(w, h, 16, 8) {
				return mb16_2x2
			}
			return mb16_2x8_4
		}
		return mb8_4x8_4
	}

	quantize := func(data []int16, quantizeTable QuantizationTable) []int8 {
		result := make([]int8, len(data))
		for i, q := range quantizeTable.Get() {
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
	dequantize := func(data []int8, quantizeTable QuantizationTable) []int16 {
		result := make([]int16, len(data))
		for i, q := range quantizeTable.Get() {
			result[i] = int16(data[i]) * q
		}
		return result
	}

	d := make(dict)
	acEncBuf := bytes.NewBuffer(nil)
	acEncode := func(out io.Writer, ac []int8, size int) error {
		defer acEncBuf.Reset()

		// int8 -> uint8
		data := unsafe.Slice((*uint8)(unsafe.Pointer(&ac[0])), len(ac))
		if err := runlength.NewEncoder(acEncBuf).Encode(data); err != nil {
			return errors.WithStack(err)
		}
		encoded := acEncBuf.Bytes()
		if len(encoded) < size { // no compress
			if _, err := out.Write(noCompress.Bytes()); err != nil {
				return errors.WithStack(err)
			}
			if _, err := acEncBuf.WriteTo(out); err != nil {
				return errors.WithStack(err)
			}
			return nil
		}

		// compress
		if _, err := out.Write(rleCompress.Bytes()); err != nil {
			return errors.WithStack(err)
		}
		chunks := uint16(len(encoded) + 3/4)
		if err := binary.Write(out, binary.BigEndian, chunks); err != nil {
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
	acDecBuf := bytes.NewBuffer(nil)
	acDecode := func(in io.Reader, size int) ([]int8, error) {
		defer acDecBuf.Reset()

		compressMode := compressMode(0)
		if err := binary.Read(in, binary.BigEndian, &compressMode); err != nil {
			return nil, errors.WithStack(err)
		}
		if compressMode == noCompress {
			b, err := runlength.NewDecoder().Decode(in)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			// uint8 -> int8
			ac := unsafe.Slice((*int8)(unsafe.Pointer(&b[0])), len(b))
			return ac, nil
		}

		chunks := uint16(0)
		if err := binary.Read(in, binary.BigEndian, &chunks); err != nil {
			return nil, errors.WithStack(err)
		}
		for i := uint16(0); i < chunks; i += 1 {
			dictIndex := uint16(0)
			if err := binary.Read(in, binary.BigEndian, &dictIndex); err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				return nil, errors.WithStack(err)
			}
			k, ok := d.Search(dictIndex)
			if ok != true {
				panic(fmt.Sprintf("idx: %d not ound", dictIndex))
			}
			if _, err := acDecBuf.Write([]byte{k[0], k[1], k[2], k[3]}); err != nil {
				return nil, errors.WithStack(err)
			}
		}

		b, err := runlength.NewDecoder().Decode(bytes.NewReader(acDecBuf.Bytes()))
		if err != nil {
			return nil, errors.WithStack(err)
		}
		// uint8 -> int8
		ac := unsafe.Slice((*int8)(unsafe.Pointer(&b[0])), len(b))
		return ac, nil
	}

	transform := func(out io.Writer, qt QuantizationTable, data [][]int16, size int) ([]int16, error) {
		dc := make([]int16, size)
		for i := 0; i < size; i += 1 {
			wht.Transform(data[i])
			dc[i] = data[i][0]
		}
		wht.Transform(dc)

		qACList := make([][]int8, size)
		for i := 0; i < size; i += 1 {
			qACList[i] = quantize(data[i], qt)
		}

		zigzag := wht.Zigzag(qACList)
		if err := acEncode(out, zigzag, size); err != nil {
			return nil, errors.WithStack(err)
		}
		return dc, nil
	}

	invert := func(dc []int16, in io.Reader, qt QuantizationTable, size int) ([][]int16, error) {
		zigzag, err := acDecode(in, size)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		acPlanes := wht.Unzigzag(zigzag, size)
		wht.Invert(dc)

		acList := make([][]int16, size)
		for i := 0; i < size; i += 1 {
			acList[i] = dequantize(acPlanes[i], qt)
			acList[i][0] = dc[i]
			wht.Invert(acList[i])
		}

		return acList, nil
	}

	dcEnc, err := createDPCMEncode()
	if err != nil {
		panic(fmt.Sprintf("%+v", err))
	}
	bufY := make([]*bytes.Buffer, 0)
	bufCb := make([]*bytes.Buffer, 0)
	bufCr := make([]*bytes.Buffer, 0)

	transformHandlerFunc := func(w, h int, size int) error {
		qtPair := qtBySize(size)
		lumaQT, chromaQT := qtPair[0], qtPair[1]
		rowsY := make([][]int16, size)
		rowsCb := make([][]int16, size)
		rowsCr := make([][]int16, size)
		for i := 0; i < size; i += 1 {
			y, cb, cr := rowN(w, h+i, size)
			rowsY[i] = y
			rowsCb[i] = cb
			rowsCr[i] = cr
		}
		dataY := bytes.NewBuffer(make([]byte, 0, size*size))
		dcY, err := transform(dataY, lumaQT, rowsY, size)
		if err != nil {
			return errors.WithStack(err)
		}
		if err := dcEnc.Write(dcY); err != nil {
			return errors.WithStack(err)
		}

		dataCb := bytes.NewBuffer(make([]byte, 0, size*size))
		dcCb, err := transform(dataCb, chromaQT, rowsCb, size)
		if err != nil {
			return errors.WithStack(err)
		}
		if err := dcEnc.Write(dcCb); err != nil {
			return errors.WithStack(err)
		}

		dataCr := bytes.NewBuffer(make([]byte, 0, size*size))
		dcCr, err := transform(dataCr, chromaQT, rowsCr, size)
		if err != nil {
			return errors.WithStack(err)
		}
		if err := dcEnc.Write(dcCr); err != nil {
			return errors.WithStack(err)
		}

		bufY = append(bufY, dataY)
		bufCb = append(bufCb, dataCb)
		bufCr = append(bufCr, dataCr)
		return nil
	}

	invertHandlerFunc := func(w, h int, size int, dcY, dcCb, dcCr []int16) error {
		qtPair := qtBySize(size)
		lumaQT, chromaQT := qtPair[0], qtPair[1]
		dataY := bufY[0]
		dataCb := bufCb[0]
		dataCr := bufCr[0]
		bufY = bufY[1:]
		bufCb = bufCb[1:]
		bufCr = bufCr[1:]

		acY := bytes.NewReader(dataY.Bytes())
		acCb := bytes.NewReader(dataCb.Bytes())
		acCr := bytes.NewReader(dataCr.Bytes())

		yPlanes, err := invert(dcY, acY, lumaQT, size)
		if err != nil {
			return errors.WithStack(err)
		}
		cbPlanes, err := invert(dcCb, acCb, chromaQT, size)
		if err != nil {
			return errors.WithStack(err)
		}
		crPlanes, err := invert(dcCr, acCr, chromaQT, size)
		if err != nil {
			return errors.WithStack(err)
		}

		for i := 0; i < size; i += 1 {
			setRowN(w, h+i, size, yPlanes[i], cbPlanes[i], crPlanes[i])
		}
		return nil
	}

	mbSeq := make([]uint8, 0)
	t := time.Now()
	b := ycbcr.Bounds()
	dx, dy := b.Dx(), b.Dy()
	for h := 0; h < dy; h += 32 {
		for w := 0; w < dx; w += 32 {
			mb := calcMacroblock(w, h)
			parts := mbPartitoins[mb]

			for _, p := range parts {
				blockX := w + p.X
				blockY := h + p.Y
				blockSize := p.Size
				if (dx < blockX+blockSize) || (dy < blockY+blockSize) {
					continue
				}
				if err := transformHandlerFunc(w+p.X, h+p.Y, p.Size); err != nil {
					panic(fmt.Sprintf("%+v", err))
				}
			}
			mbSeq = append(mbSeq, uint8(mb))
		}
	}

	if err := dcEnc.Close(); err != nil {
		panic(fmt.Sprintf("%+v", err))
	}

	original := len(ycbcr.Y) + len(ycbcr.Cb) + len(ycbcr.Cr)
	compressedSize := 0
	compressedSize += dcEnc.Len()
	for _, b := range bufY {
		compressedSize += b.Len()
	}
	for _, b := range bufCb {
		compressedSize += b.Len()
	}
	for _, b := range bufCr {
		compressedSize += b.Len()
	}

	mbSeqRLEBuf := bytes.NewBuffer(nil)
	if err := runlength.NewEncoder(mbSeqRLEBuf).Encode(mbSeq); err != nil {
		panic(fmt.Sprintf("%+v", err))
	}
	compressedSize += mbSeqRLEBuf.Len()

	dictDump := bytes.NewBuffer(nil)
	d.Dump(dictDump)
	compDict := bytes.NewBuffer(nil)
	huff, err := flate.NewWriter(compDict, flate.HuffmanOnly)
	if err != nil {
		panic(fmt.Sprintf("%+v", err))
	}
	if _, err := huff.Write(dictDump.Bytes()); err != nil {
		panic(fmt.Sprintf("%+v", err))
	}
	huff.Close()

	compressedSize += compDict.Len()
	fmt.Printf(
		"elapse=%s compressed %3.2f%% (dict size %3.2fKB %3.2f%% -> %3.2fKB %3.2f%%)\n",
		time.Since(t),
		(float64(compressedSize)/float64(original))*100,
		float64(dictDump.Len())/1024.0,
		(float64(dictDump.Len())/float64(compressedSize))*100,
		float64(compDict.Len())/1024.0,
		(float64(compDict.Len())/float64(compressedSize))*100,
	)

	dcDec, err := createDPCMDecode(bytes.NewReader(dcEnc.Bytes()))
	if err != nil {
		panic(fmt.Sprintf("%+v", err))
	}

	for h := 0; h < dy; h += 32 {
		for w := 0; w < dx; w += 32 {
			mb := macroblock(mbSeq[0])
			mbSeq = mbSeq[1:]
			parts := mbPartitoins[mb]

			for _, p := range parts {
				blockX := w + p.X
				blockY := h + p.Y
				blockSize := p.Size
				if (dx < blockX+blockSize) || (dy < blockY+blockSize) {
					continue
				}
				dcY, err := dcDec.ReadSize(blockSize)
				if err != nil {
					panic(fmt.Sprintf("%+v", err))
				}
				dcCb, err := dcDec.ReadSize(blockSize)
				if err != nil {
					panic(fmt.Sprintf("%+v", err))
				}
				dcCr, err := dcDec.ReadSize(blockSize)
				if err != nil {
					panic(fmt.Sprintf("%+v", err))
				}
				if err := invertHandlerFunc(w+p.X, h+p.Y, p.Size, dcY, dcCb, dcCr); err != nil {
					panic(fmt.Sprintf("%+v", err))
				}
			}
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
