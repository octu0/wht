package main

func quantizeLow(block [][]int16, size uint16, scale int) {
	quantize(block, size, 8)
}

func quantizeMid(block [][]int16, size uint16, scale int) {
	quantize(block, size, scale+2)
}

func quantizeHigh(block [][]int16, size uint16, scale int) {
	quantize(block, size, scale+5)
}

func quantize(block [][]int16, size uint16, shift int) {
	for y := uint16(0); y < size; y += 1 {
		for x := uint16(0); x < size; x += 1 {
			v := int32(block[y][x])
			off := int32(1 << (shift - 1))
			if 0 <= v {
				block[y][x] = int16((v + off) >> shift)
			} else {
				block[y][x] = int16(-((-v + off) >> shift))
			}
		}
	}
}

func dequantizeLow(block [][]int16, size uint16, scale int) {
	dequantize(block, size, 8)
}

func dequantizeMid(block [][]int16, size uint16, scale int) {
	dequantize(block, size, scale+2)
}

func dequantizeHigh(block [][]int16, size uint16, scale int) {
	dequantize(block, size, scale+5)
}

func dequantize(block [][]int16, size uint16, shift int) {
	for y := uint16(0); y < size; y += 1 {
		for x := uint16(0); x < size; x += 1 {
			block[y][x] = block[y][x] << shift
		}
	}
}
