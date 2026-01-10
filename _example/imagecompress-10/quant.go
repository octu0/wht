package main

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
		24, 24, 32,
	}
	lumaQuantizationTable16 = []int16{
		16, 16, 16, 16,
		24, 24, 24, 32,
		32, 32, 48, 48,
		64, 80, 96,
	}
	lumaQuantizationTable32 = []int16{
		16, 16, 16, 16,
		16, 16, 16, 16,
		16, 16, 24, 24,
		24, 24, 24, 24,
		24, 24, 32, 32,
		32, 32, 32, 32,
		48, 48, 48, 64,
		64, 80, 96,
	}
	chromaQuantizationTable8 = []int16{
		64, 64, 64, 64,
		100, 128, 192,
	}
	chromaQuantizationTable16 = []int16{
		64, 64, 80, 90,
		100, 112, 128, 160,
		176, 192, 208, 208,
		220, 220, 240,
	}
	chromaQuantizationTable32 = []int16{
		64, 64, 64, 64,
		64, 64, 64, 90,
		90, 90, 90, 90,
		100, 100, 110, 110,
		128, 128, 144, 160,
		176, 192, 208, 208,
		220, 220, 240, 240,
		240, 240, 240,
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

func dcQuantize(data []int16, quantizeTable QuantizationTable) {
	for i, q := range quantizeTable.Get() {
		data[i] = data[i] / q
	}
}

func dcDequantize(data []int16, quantizeTable QuantizationTable) {
	for i, q := range quantizeTable.Get() {
		data[i] = data[i] * q
	}
}

func acQuantize(data []int16, quantizeTable QuantizationTable, scale int) []int8 {
	if scale < 1 {
		scale = 1
	}
	result := make([]int8, len(data))
	for i, q := range quantizeTable.Get() {
		v := data[i] / (q * int16(scale))
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

func acDequantize(data []int8, quantizeTable QuantizationTable, scale int) []int16 {
	if scale < 1 {
		scale = 1
	}
	result := make([]int16, len(data))
	for i, q := range quantizeTable.Get() {
		result[i] = int16(data[i]) * (q * int16(scale))
	}
	return result
}
