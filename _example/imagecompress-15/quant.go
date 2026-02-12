package main

func quantizeLow(block [][]int16, size uint16, scale int) {
	quantize(block, size, scale+2)
}

func quantizeMid(block [][]int16, size uint16, scale int) {
	quantize(block, size, scale+3)
}

func quantizeHigh(block [][]int16, size uint16, scale int) {
	quantize(block, size, scale+5)
}

func quantize(data [][]int16, size uint16, scale int) {
	for y := uint16(0); y < size; y += 1 {
		for x := uint16(0); x < size; x += 1 {
			v := int32(data[y][x])
			off := int32(1 << (scale - 1))
			if 0 <= v {
				data[y][x] = int16((v + off) >> scale)
			} else {
				data[y][x] = int16(-1 * ((-1*v + off) >> scale))
			}
		}
	}
}

func dequantizeLow(block [][]int16, size uint16, scale int) {
	dequantize(block, size, scale+2)
}

func dequantizeMid(block [][]int16, size uint16, scale int) {
	dequantize(block, size, scale+3)
}

func dequantizeHigh(block [][]int16, size uint16, scale int) {
	dequantize(block, size, scale+5)
}

func dequantize(data [][]int16, size uint16, scale int) {
	for y := uint16(0); y < size; y += 1 {
		for x := uint16(0); x < size; x += 1 {
			data[y][x] <<= scale
		}
	}
}
