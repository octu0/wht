package main

// DWT 5/3 の特性に合わせ、ビットシフトベースの量子化を行う
// scale は RateController から渡されるベースシフト量

func quantizeBlock(block [][]int16, size int, scale int) {
	baseShift := scale
	if baseShift < 0 {
		baseShift = 0
	}

	half := size / 2
	quarter := size / 4
	if size < 16 {
		quarter = 0
	}

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			// デフォルトは高周波 (Level 1)
			shift := baseShift + 5

			if x < half && y < half {
				// Level 2 サブバンド内
				shift = baseShift + 2
				if quarter > 0 && x < quarter && y < quarter {
					// LL2 (最も低周波な成分)
					shift = baseShift
				}
			}
			if shift < 0 {
				shift = 0
			}

			if shift == 0 {
				continue
			}

			v := int32(block[y][x])
			off := int32(1 << (shift - 1))
			if v >= 0 {
				block[y][x] = int16((v + off) >> shift)
			} else {
				block[y][x] = int16(-((-v + off) >> shift))
			}
		}
	}
}

func dequantizeBlock(block [][]int16, size int, scale int) {
	baseShift := scale
	if baseShift < 0 {
		baseShift = 0
	}

	half := size / 2
	quarter := size / 4
	if size < 16 {
		quarter = 0
	}

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			shift := baseShift + 5
			if x < half && y < half {
				shift = baseShift + 2
				if quarter > 0 && x < quarter && y < quarter {
					shift = baseShift
				}
			}
			if shift < 0 {
				shift = 0
			}

			block[y][x] = block[y][x] << shift
		}
	}
}
