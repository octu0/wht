package main

func quantizeBlock(block [][]int16, size uint16, scale int) {
	baseShift := scale
	if baseShift < 0 {
		baseShift = 0
	}

	half := size / 2
	quarter := size / 4
	if size < 16 {
		quarter = 0
	}

	for y := uint16(0); y < size; y += 1 {
		for x := uint16(0); x < size; x += 1 {
			// デフォルトは高周波 (Level 1)
			shift := baseShift + 5

			if x < half && y < half {
				// Level 2 サブバンド内
				shift = baseShift + 2
				if 0 < quarter && x < quarter && y < quarter {
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

func dequantizeBlock(block [][]int16, size uint16, scale int) {
	baseShift := scale
	if baseShift < 0 {
		baseShift = 0
	}

	half := size / 2
	quarter := size / 4
	if size < 16 {
		quarter = 0
	}

	for y := uint16(0); y < size; y += 1 {
		for x := uint16(0); x < size; x += 1 {
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
