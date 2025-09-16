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
	dcQuantizationTable8 = []int16{
		16, 16, 16, 16,
		16, 16, 64, 64,
	}
	dcQuantizationTable16 = []int16{
		16, 16, 16, 16,
		16, 16, 16, 16,
		32, 32, 32, 32,
		64, 64, 64, 64,
	}
	dcQuantizationTable32 = []int16{
		16, 16, 16, 16,
		16, 16, 16, 16,
		24, 24, 24, 24,
		24, 24, 24, 24,
		32, 32, 32, 32,
		32, 32, 32, 32,
		64, 64, 64, 64,
		64, 64, 64, 64,
	}
	lumaQuantizationTable8 = []int16{
		16, 16, 16, 16,
		24, 24, 32, 32,
	}
	lumaQuantizationTable16 = []int16{
		16, 16, 16, 16,
		24, 24, 24, 32,
		32, 32, 48, 48,
		64, 80, 96, 96,
	}
	lumaQuantizationTable32 = []int16{
		16, 16, 16, 16,
		16, 16, 16, 16,
		16, 16, 24, 24,
		24, 24, 24, 24,
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
	dcQT8      = QuantizationTable(dcQuantizationTable8)
	dcQT16     = QuantizationTable(dcQuantizationTable16)
	dcQT32     = QuantizationTable(dcQuantizationTable32)
	lumaQT8    = QuantizationTable(lumaQuantizationTable8)
	lumaQT16   = QuantizationTable(lumaQuantizationTable16)
	lumaQT32   = QuantizationTable(lumaQuantizationTable32)
	chromaQT8  = QuantizationTable(chromaQuantizationTable8)
	chromaQT16 = QuantizationTable(chromaQuantizationTable16)
	chromaQT32 = QuantizationTable(chromaQuantizationTable32)
)

func qtBySize(size int) [3]QuantizationTable {
	switch macroblock(size) {
	case 8:
		return [3]QuantizationTable{dcQT8, lumaQT8, chromaQT8}
	case 16:
		return [3]QuantizationTable{dcQT16, lumaQT16, chromaQT16}
	case 32:
		return [3]QuantizationTable{dcQT32, lumaQT32, chromaQT32}
	}
	panic(fmt.Sprintf("size:%d not found", size))
}

const (
	maxIndexSize uint16 = 65535
)

type (
	dcDictKey [2]int16
	acDictKey [4]uint8
)

type dcDict struct {
	lastIndex uint16
	keyIndex  map[dcDictKey]uint16
	indexKey  map[uint16]dcDictKey
}

func (d *dcDict) HasCapacity(size uint16) bool {
	if (d.lastIndex + size) < maxIndexSize {
		return true
	}
	return false
}

func (d *dcDict) Add(key dcDictKey) uint16 {
	idx, ok := d.keyIndex[key]
	if ok {
		return idx
	}
	d.keyIndex[key] = d.lastIndex
	d.indexKey[d.lastIndex] = key
	d.lastIndex += 1
	return d.keyIndex[key]
}

func (d *dcDict) Get(idx uint16) (dcDictKey, bool) {
	key, ok := d.indexKey[idx]
	if ok != true {
		return dcDictKey{}, false
	}
	return key, true
}

func (d *dcDict) WriteTo(out io.Writer) error {
	size := len(d.keyIndex)
	if err := binary.Write(out, binary.BigEndian, uint16(size)); err != nil {
		return errors.WithStack(err)
	}
	for k, v := range d.keyIndex {
		if err := binary.Write(out, binary.BigEndian, k[0]); err != nil {
			return errors.WithStack(err)
		}
		if err := binary.Write(out, binary.BigEndian, k[1]); err != nil {
			return errors.WithStack(err)
		}
		if err := binary.Write(out, binary.BigEndian, v); err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

func (d *dcDict) ReadFrom(in io.Reader) error {
	size := uint16(0)
	if err := binary.Read(in, binary.BigEndian, &size); err != nil {
		return errors.WithStack(err)
	}
	for i := uint16(0); i < size; i += 1 {
		k0, k1 := int16(0), int16(0)
		dictIndex := uint16(0)
		if err := binary.Read(in, binary.BigEndian, &k0); err != nil {
			return errors.WithStack(err)
		}
		if err := binary.Read(in, binary.BigEndian, &k1); err != nil {
			return errors.WithStack(err)
		}
		if err := binary.Read(in, binary.BigEndian, &dictIndex); err != nil {
			return errors.WithStack(err)
		}
		key := dcDictKey{k0, k1}
		d.keyIndex[key] = dictIndex
		d.indexKey[dictIndex] = key
	}
	return nil
}

func newDCDict() *dcDict {
	return &dcDict{
		lastIndex: 0,
		keyIndex:  make(map[dcDictKey]uint16),
		indexKey:  make(map[uint16]dcDictKey),
	}
}

type acDict struct {
	lastIndex uint16
	keyIndex  map[acDictKey]uint16
	indexKey  map[uint16]acDictKey
}

func (d *acDict) HasCapacity(size uint16) bool {
	if (d.lastIndex + size) < maxIndexSize {
		return true
	}
	return false
}

func (d *acDict) Add(key acDictKey) uint16 {
	idx, ok := d.keyIndex[key]
	if ok {
		return idx
	}
	d.keyIndex[key] = d.lastIndex
	d.indexKey[d.lastIndex] = key
	d.lastIndex += 1
	return d.keyIndex[key]
}

func (d *acDict) Get(idx uint16) (acDictKey, bool) {
	key, ok := d.indexKey[idx]
	if ok != true {
		return acDictKey{}, false
	}
	return key, true
}

func (d *acDict) WriteTo(out io.Writer) error {
	size := len(d.keyIndex)
	if err := binary.Write(out, binary.BigEndian, uint16(size)); err != nil {
		return errors.WithStack(err)
	}
	for k, v := range d.keyIndex {
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

func (d *acDict) ReadFrom(in io.Reader) error {
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
		key := acDictKey{k0, k1, k2, k3}
		d.keyIndex[key] = dictIndex
		d.indexKey[dictIndex] = key
	}
	return nil
}

func newACDict() *acDict {
	return &acDict{
		lastIndex: 0,
		keyIndex:  make(map[acDictKey]uint16),
		indexKey:  make(map[uint16]acDictKey),
	}
}

func main() {
	ycbcr, err := pngToYCbCr(srcPng)
	if err != nil {
		panic(fmt.Sprintf("%+v", err))
	}
	type headtailFunc func(w, h int, n int) (uint8, uint8)
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
	type rowFunc func(x, y int, size int) []int16
	rowY := func(x, y int, size int) []int16 {
		plane := make([]int16, size)
		for i := 0; i < size; i += 1 {
			// - 128 = level shift
			plane[i] = int16(ycbcr.Y[ycbcr.YOffset(x+i, y)]) - 128
		}
		return plane
	}
	rowCb := func(x, y int, size int) []int16 {
		plane := make([]int16, size)
		for i := 0; i < size; i += 1 {
			// 4:4:4 -> 4:2:0 への簡易間引き
			v := int16(ycbcr.Cb[ycbcr.COffset((x+i)*2, y*2)]) - 128
			plane[i] = v
		}
		return plane
	}
	rowCr := func(x, y int, size int) []int16 {
		plane := make([]int16, size)
		for i := 0; i < size; i += 1 {
			v := int16(ycbcr.Cr[ycbcr.COffset((x+i)*2, y*2)]) - 128
			plane[i] = v
		}
		return plane
	}
	newImg := image.NewYCbCr(ycbcr.Bounds(), image.YCbCrSubsampleRatio420)
	type setRowFunc func(x, y int, size int, plane []int16)
	setRowY := func(x, y int, size int, plane []int16) {
		defer func() {
			if r := recover(); r != nil {
			}
		}()
		for i := 0; i < size; i += 1 {
			newImg.Y[newImg.YOffset(x+i, y)] = clampU8(plane[i] + 128)
		}
	}
	setRowCb := func(x, y int, size int, plane []int16) {
		defer func() {
			if r := recover(); r != nil {
			}
		}()
		for i := 0; i < size; i += 1 {
			newImg.Cb[newImg.COffset((x+i)*2, y*2)] = clampU8(plane[i] + 128)
		}
	}
	setRowCr := func(x, y int, size int, plane []int16) {
		defer func() {
			if r := recover(); r != nil {
			}
		}()
		for i := 0; i < size; i += 1 {
			newImg.Cr[newImg.COffset((x+i)*2, y*2)] = clampU8(plane[i] + 128)
		}
	}

	similarblock := func(w, h int, size int, headtail headtailFunc) bool {
		h0, t0 := headtail(w, h, size)
		h1, t1 := headtail(w, h+(size-1), size)

		mi := min(min(h0, h1), min(t0, t1))
		ma := max(max(h0, h1), max(t0, t1))
		if (ma - mi) < 32 { // 32/255 = 0.1254
			return true
		}
		return false
	}
	similarblockNeighbor := func(w, h int, size, next int, headtail headtailFunc) bool {
		h0, t0 := headtail(w, h, size)
		h1, t1 := headtail(w, h+(size-1), size)
		h2, t2 := headtail(w, h+(size+next-1), size)

		mi := min(min(min(h0, h1), min(t0, t1)), min(h2, t2))
		ma := max(max(max(h0, h1), max(t0, t1)), max(h2, t2))
		if (ma - mi) < 16 { // 16/255 = 0.0627
			return true
		}
		return false
	}
	calcMacroblockY := func(w, h int) macroblock {
		if similarblock(w, h, 16, headtailY) {
			if similarblockNeighbor(w, h, 16, 8, headtailY) {
				return mb16_2x2
			}
			return mb16_2x8_4
		}
		return mb8_4x8_4
	}
	calcMacroblockCb := func(w, h int) macroblock {
		return mb8_4x8_4
	}
	calcMacroblockCr := func(w, h int) macroblock {
		return mb8_4x8_4
	}

	dcQuantize := func(data []int16, quantizeTable QuantizationTable) {
		for i, q := range quantizeTable.Get() {
			data[i] = data[i] / q
		}
	}
	dcDequantize := func(data []int16, quantizeTable QuantizationTable) {
		for i, q := range quantizeTable.Get() {
			data[i] = data[i] * q
		}
	}
	acQuantize := func(data []int16, quantizeTable QuantizationTable) []int8 {
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
	acDequantize := func(data []int8, quantizeTable QuantizationTable) []int16 {
		result := make([]int16, len(data))
		for i, q := range quantizeTable.Get() {
			result[i] = int16(data[i]) * q
		}
		return result
	}

	dcDict := newDCDict()
	dcEncode := func(out io.Writer, dc []int16, size int) error {
		chunks := uint16(size / 2)

		if dcDict.HasCapacity(chunks) != true {
			if _, err := out.Write(noCompress.Bytes()); err != nil {
				return errors.WithStack(err)
			}
			for _, v := range dc {
				if err := binary.Write(out, binary.BigEndian, v); err != nil {
					return errors.WithStack(err)
				}
			}
			return nil
		}

		if _, err := out.Write(rleCompress.Bytes()); err != nil {
			return errors.WithStack(err)
		}

		dcdc := dc[0]
		dcs := append(dc[1:], 0)
		if err := binary.Write(out, binary.BigEndian, dcdc); err != nil {
			return errors.WithStack(err)
		}
		for i := 0; i < size; i += 2 {
			dictIndex := dcDict.Add(dcDictKey{dcs[i], dcs[i+1]})
			if err := binary.Write(out, binary.BigEndian, dictIndex); err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	}
	dcDecode := func(in io.Reader, size int) ([]int16, error) {
		compressMode := compressMode(0)
		if err := binary.Read(in, binary.BigEndian, &compressMode); err != nil {
			return nil, errors.WithStack(err)
		}
		if compressMode == noCompress {
			dc := make([]int16, size)
			for i := 0; i < size; i += 1 {
				v := int16(0)
				if err := binary.Read(in, binary.BigEndian, &v); err != nil {
					return nil, errors.WithStack(err)
				}
				dc[i] = v
			}
			return dc, nil
		}

		dc := make([]int16, 0, size)
		dcdc := int16(0)
		if err := binary.Read(in, binary.BigEndian, &dcdc); err != nil {
			return nil, errors.WithStack(err)
		}
		dc = append(dc, dcdc)

		chunks := uint16(size / 2)
		for i := uint16(0); i < chunks; i += 1 {
			dictIndex := uint16(0)
			if err := binary.Read(in, binary.BigEndian, &dictIndex); err != nil {
				return nil, errors.WithStack(err)
			}
			key, ok := dcDict.Get(dictIndex)
			if ok != true {
				panic(fmt.Sprintf("idx: %d not ound", dictIndex))
			}
			dc = append(dc, key[0], key[1])
		}
		return dc[:size], nil
	}

	acDict := newACDict()
	acEncBuf := bytes.NewBuffer(nil)
	acEncode := func(out io.Writer, ac []int8, size int) error {
		defer acEncBuf.Reset()

		// int8 -> uint8
		data := unsafe.Slice((*uint8)(unsafe.Pointer(&ac[0])), len(ac))
		if err := runlength.NewEncoder(acEncBuf).Encode(data); err != nil {
			return errors.WithStack(err)
		}
		encoded := acEncBuf.Bytes()
		if len(encoded) < (size / 4) { // no compress
			if _, err := out.Write(noCompress.Bytes()); err != nil {
				return errors.WithStack(err)
			}
			if _, err := acEncBuf.WriteTo(out); err != nil {
				return errors.WithStack(err)
			}
			return nil
		}

		chunks := uint16(len(encoded) + 3/4)
		if acDict.HasCapacity(chunks) != true { // no compress
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
			dictIndex := acDict.Add(acDictKey{k0, k1, k2, k3})
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
			key, ok := acDict.Get(dictIndex)
			if ok != true {
				panic(fmt.Sprintf("idx: %d not ound", dictIndex))
			}
			if _, err := acDecBuf.Write([]byte{key[0], key[1], key[2], key[3]}); err != nil {
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

	srcbit := ycbcr.Bounds().Dx() * ycbcr.Bounds().Dy() * 8
	maxbit := 500 * 1000
	fmt.Printf("src %d bit\n", srcbit)
	fmt.Printf("target %d bit = %3.2f%%\n", maxbit, (float64(maxbit)/float64(srcbit))*100)
	maxbyte := maxbit / 8
	avgblock := int16(maxbyte / 64)

	transform := func(out io.Writer, dcQT, acQT QuantizationTable, data [][]int16, size int) error {
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
			qACList[i] = acQuantize(data[i], acQT)
		}

		zigzag := wht.Zigzag(qACList)

		// CBR
		sum := int16(0)
		for i, v := range zigzag {
			if v < 0 {
				sum -= int16(v)
			} else {
				sum += int16(v)
			}
			if avgblock < sum {
				zigzag[i] = 0
			}
		}

		if err := acEncode(out, zigzag, size); err != nil {
			return errors.WithStack(err)
		}
		return nil
	}

	invert := func(in io.Reader, dcQT, acQT QuantizationTable, size int) ([][]int16, error) {
		dc, err := dcDecode(in, size)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		zigzag, err := acDecode(in, size)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		acPlanes := wht.Unzigzag(zigzag, size)

		dcDequantize(dc, dcQT)
		wht.Invert(dc)

		acList := make([][]int16, size)
		for i := 0; i < size; i += 1 {
			acList[i] = acDequantize(acPlanes[i], acQT)
			acList[i][0] = dc[i]
			wht.Invert(acList[i])
		}

		return acList, nil
	}

	bufY := make([]*bytes.Buffer, 0)
	bufCb := make([]*bytes.Buffer, 0)
	bufCr := make([]*bytes.Buffer, 0)

	transformHandlerFunc := func(w, h int, size int, dcQT, acQT QuantizationTable, rowN rowFunc) (*bytes.Buffer, error) {
		rows := make([][]int16, size)
		for i := 0; i < size; i += 1 {
			rows[i] = rowN(w, h+i, size)
		}
		data := bytes.NewBuffer(make([]byte, 0, size*size))
		if err := transform(data, dcQT, acQT, rows, size); err != nil {
			return nil, errors.WithStack(err)
		}
		return data, nil
	}

	invertHandlerFunc := func(in io.Reader, w, h int, size int, dcQT, acQT QuantizationTable, setRow setRowFunc) error {
		planes, err := invert(in, dcQT, acQT, size)
		if err != nil {
			return errors.WithStack(err)
		}

		for i := 0; i < size; i += 1 {
			setRow(w, h+i, size, planes[i])
		}
		return nil
	}

	mbYSeq := make([]uint8, 0)
	mbCbSeq := make([]uint8, 0)
	mbCrSeq := make([]uint8, 0)
	t := time.Now()
	b := ycbcr.Bounds()
	dx, dy := b.Dx(), b.Dy()
	for h := 0; h < dy; h += 32 {
		for w := 0; w < dx; w += 32 {
			mb := calcMacroblockY(w, h)
			parts := mbPartitoins[mb]

			for _, p := range parts {
				blockX := w + p.X
				blockY := h + p.Y
				blockSize := p.Size
				if (dx < blockX+blockSize) || (dy < blockY+blockSize) {
					continue
				}
				qtPair := qtBySize(p.Size)
				dcQT, lumaQT := qtPair[0], qtPair[1]
				data, err := transformHandlerFunc(blockX, blockY, p.Size, dcQT, lumaQT, rowY)
				if err != nil {
					panic(fmt.Sprintf("%+v", err))
				}
				bufY = append(bufY, data)
			}
			mbYSeq = append(mbYSeq, uint8(mb))
		}
	}
	for h := 0; h < dy/2; h += 32 {
		for w := 0; w < dx/2; w += 32 {
			mb := calcMacroblockCb(w, h)
			parts := mbPartitoins[mb]

			for _, p := range parts {
				blockX := w + p.X
				blockY := h + p.Y
				blockSize := p.Size
				if (dx/2 < blockX+blockSize) || (dy/2 < blockY+blockSize) {
					continue
				}
				qtPair := qtBySize(p.Size)
				dcQT, chromaQT := qtPair[0], qtPair[2]
				data, err := transformHandlerFunc(blockX, blockY, p.Size, dcQT, chromaQT, rowCb)
				if err != nil {
					panic(fmt.Sprintf("%+v", err))
				}
				bufCb = append(bufCb, data)
			}
			mbCbSeq = append(mbCbSeq, uint8(mb))
		}
	}
	for h := 0; h < dy/2; h += 32 {
		for w := 0; w < dx/2; w += 32 {
			mb := calcMacroblockCr(w, h)
			parts := mbPartitoins[mb]

			for _, p := range parts {
				blockX := w + p.X
				blockY := h + p.Y
				blockSize := p.Size
				if (dx/2 < blockX+blockSize) || (dy/2 < blockY+blockSize) {
					continue
				}
				qtPair := qtBySize(p.Size)
				dcQT, chromaQT := qtPair[0], qtPair[2]
				data, err := transformHandlerFunc(blockX, blockY, p.Size, dcQT, chromaQT, rowCr)
				if err != nil {
					panic(fmt.Sprintf("%+v", err))
				}
				bufCr = append(bufCr, data)
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

	dcDictDump := bytes.NewBuffer(nil)
	dcDict.WriteTo(dcDictDump)
	compDCDict := bytes.NewBuffer(nil)
	huffDC, err := flate.NewWriter(compDCDict, flate.HuffmanOnly)
	if err != nil {
		panic(fmt.Sprintf("%+v", err))
	}
	if _, err := huffDC.Write(dcDictDump.Bytes()); err != nil {
		panic(fmt.Sprintf("%+v", err))
	}
	huffDC.Close()

	acDictDump := bytes.NewBuffer(nil)
	acDict.WriteTo(acDictDump)
	compACDict := bytes.NewBuffer(nil)
	huffAC, err := flate.NewWriter(compACDict, flate.HuffmanOnly)
	if err != nil {
		panic(fmt.Sprintf("%+v", err))
	}
	if _, err := huffAC.Write(acDictDump.Bytes()); err != nil {
		panic(fmt.Sprintf("%+v", err))
	}
	huffAC.Close()

	compressedSize += compDCDict.Len()
	compressedSize += compACDict.Len()
	fmt.Printf(
		"elapse=%s %3.2fKB -> %3.2fKB compressed %3.2f%%\n",
		time.Since(t),
		float64(original)/1024.0,
		float64(compressedSize)/1024.0,
		(float64(compressedSize)/float64(original))*100,
	)
	fmt.Printf(
		"dc: dict size %3.2fKB %3.2f%% -> %3.2fKB %3.2f%%\n",
		float64(dcDictDump.Len())/1024.0,
		(float64(dcDictDump.Len())/float64(compressedSize))*100,
		float64(compDCDict.Len())/1024.0,
		(float64(compDCDict.Len())/float64(compressedSize))*100,
	)
	fmt.Printf(
		"ac: dict size %3.2fKB %3.2f%% -> %3.2fKB %3.2f%%\n",
		float64(acDictDump.Len())/1024.0,
		(float64(acDictDump.Len())/float64(compressedSize))*100,
		float64(compACDict.Len())/1024.0,
		(float64(compACDict.Len())/float64(compressedSize))*100,
	)

	for h := 0; h < dy; h += 32 {
		for w := 0; w < dx; w += 32 {
			mb := macroblock(mbYSeq[0])
			mbYSeq = mbYSeq[1:]
			parts := mbPartitoins[mb]

			for _, p := range parts {
				blockX := w + p.X
				blockY := h + p.Y
				blockSize := p.Size
				if (dx < blockX+blockSize) || (dy < blockY+blockSize) {
					continue
				}
				data := bufY[0]
				bufY = bufY[1:]
				in := bytes.NewReader(data.Bytes())

				qtPair := qtBySize(p.Size)
				dcQT, lumaQT := qtPair[0], qtPair[1]
				if err := invertHandlerFunc(in, blockX, blockY, p.Size, dcQT, lumaQT, setRowY); err != nil {
					panic(fmt.Sprintf("%+v", err))
				}
			}
		}
	}
	for h := 0; h < dy/2; h += 32 {
		for w := 0; w < dx/2; w += 32 {
			mb := macroblock(mbCbSeq[0])
			mbCbSeq = mbCbSeq[1:]
			parts := mbPartitoins[mb]

			for _, p := range parts {
				blockX := w + p.X
				blockY := h + p.Y
				blockSize := p.Size
				if (dx/2 < blockX+blockSize) || (dy/2 < blockY+blockSize) {
					continue
				}
				data := bufCb[0]
				bufCb = bufCb[1:]
				in := bytes.NewReader(data.Bytes())

				qtPair := qtBySize(p.Size)
				dcQT, chromaQT := qtPair[0], qtPair[2]
				if err := invertHandlerFunc(in, blockX, blockY, p.Size, dcQT, chromaQT, setRowCb); err != nil {
					panic(fmt.Sprintf("%+v", err))
				}
			}
		}
	}
	for h := 0; h < dy/2; h += 32 {
		for w := 0; w < dx/2; w += 32 {
			mb := macroblock(mbCrSeq[0])
			mbCrSeq = mbCrSeq[1:]
			parts := mbPartitoins[mb]

			for _, p := range parts {
				blockX := w + p.X
				blockY := h + p.Y
				blockSize := p.Size
				if (dx/2 < blockX+blockSize) || (dy/2 < blockY+blockSize) {
					continue
				}
				data := bufCr[0]
				bufCr = bufCr[1:]
				in := bytes.NewReader(data.Bytes())

				qtPair := qtBySize(p.Size)
				dcQT, chromaQT := qtPair[0], qtPair[2]
				if err := invertHandlerFunc(in, blockX, blockY, p.Size, dcQT, chromaQT, setRowCr); err != nil {
					panic(fmt.Sprintf("%+v", err))
				}
			}
		}
	}

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
