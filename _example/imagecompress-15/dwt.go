package main

type Subbands struct {
	LL, HL, LH, HH [][]int16
	Size           uint16
}

func lift53(data []int16) {
	n := len(data)
	half := n / 2
	low := make([]int16, half)
	high := make([]int16, half)
	for i := 0; i < half; i += 1 {
		low[i] = data[2*i]
		high[i] = data[2*i+1]
	}
	for i := 0; i < half; i += 1 { // Predict
		l, r := int32(low[i]), int32(low[i])
		if i+1 < half {
			r = int32(low[i+1])
		}
		high[i] -= int16((l + r) >> 1)
	}
	for i := 0; i < half; i++ { // Update
		d, dp := int32(high[i]), int32(high[i])
		if 0 <= i-1 {
			dp = int32(high[i-1])
		}
		low[i] += int16((dp + d + 2) >> 2)
	}
	for i := 0; i < half; i += 1 {
		data[i] = low[i]
		data[half+i] = high[i]
	}
}

func invLift53(data []int16) {
	n := len(data)
	half := n / 2
	low := make([]int16, half)
	high := make([]int16, half)
	copy(low, data[:half])
	copy(high, data[half:])
	for i := 0; i < half; i++ { // Inv Update
		d, dp := int32(high[i]), int32(high[i])
		if i-1 >= 0 {
			dp = int32(high[i-1])
		}
		low[i] -= int16((dp + d + 2) >> 2)
	}
	for i := 0; i < half; i++ { // Inv Predict
		l, r := int32(low[i]), int32(low[i])
		if i+1 < half {
			r = int32(low[i+1])
		}
		high[i] += int16((l + r) >> 1)
	}
	for i := 0; i < half; i++ {
		data[2*i] = low[i]
		data[2*i+1] = high[i]
	}
}

func dwt2d(data [][]int16, size uint16) Subbands {
	for y := uint16(0); y < size; y += 1 {
		lift53(data[y])
	}

	col := make([]int16, size)
	for x := uint16(0); x < size; x += 1 {
		for y := uint16(0); y < size; y += 1 {
			col[y] = data[y][x]
		}
		lift53(col)
		for y := uint16(0); y < size; y += 1 {
			data[y][x] = col[y]
		}
	}

	half := (size + 1) / 2

	sub := Subbands{
		LL:   make([][]int16, half),
		HL:   make([][]int16, half),
		LH:   make([][]int16, half),
		HH:   make([][]int16, half),
		Size: half,
	}

	for y := uint16(0); y < half; y += 1 {
		sub.LL[y] = make([]int16, half)
		sub.HL[y] = make([]int16, half)
		sub.LH[y] = make([]int16, half)
		sub.HH[y] = make([]int16, half)
	}

	for y := uint16(0); y < half; y += 1 {
		for x := uint16(0); x < size; x += 1 {
			val := data[y][x]
			if x < half {
				sub.LL[y][x] = val // Top-Left
			} else {
				sub.HL[y][x-half] = val // Top-Right
			}
		}
	}
	for y := half; y < size; y += 1 {
		for x := uint16(0); x < size; x += 1 {
			val := data[y][x]
			if x < half {
				sub.LH[y-half][x] = val // Bottom-Left
			} else {
				sub.HH[y-half][x-half] = val // Bottom-Right
			}
		}
	}
	return sub
}

func invDwt2d(sub Subbands) [][]int16 {
	half := sub.Size
	size := sub.Size * 2

	data := make([][]int16, size)
	for y := uint16(0); y < size; y += 1 {
		data[y] = make([]int16, size)
	}

	for y := uint16(0); y < half; y += 1 {
		for x := uint16(0); x < size; x += 1 {
			if x < half {
				data[y][x] = sub.LL[y][x]
			} else {
				data[y][x] = sub.HL[y][x-half]
			}
		}
	}
	for y := half; y < size; y += 1 {
		for x := uint16(0); x < size; x += 1 {
			if x < half {
				data[y][x] = sub.LH[y-half][x]
			} else {
				data[y][x] = sub.HH[y-half][x-half]
			}
		}
	}

	col := make([]int16, size)
	for x := uint16(0); x < size; x += 1 {
		for y := uint16(0); y < size; y += 1 {
			col[y] = data[y][x]
		}
		invLift53(col)
		for y := uint16(0); y < size; y += 1 {
			data[y][x] = col[y]
		}
	}

	for y := uint16(0); y < size; y += 1 {
		invLift53(data[y])
	}
	return data
}
