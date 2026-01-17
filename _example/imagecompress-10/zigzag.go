package main

type Signed interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64
}

type coord struct {
	R, C uint8
}

func zigzag8x7[T Signed](data [][]T, result []T) {
	for i, c := range table8x7 {
		result[i] = data[c.R][c.C]
	}
}

func zigzag8x8[T Signed](data [][]T, result []T) {
	for i, c := range table8x8 {
		result[i] = data[c.R][c.C]
	}
}

func zigzag16x15[T Signed](data [][]T, result []T) {
	for i, c := range table16x15 {
		result[i] = data[c.R][c.C]
	}
}

func zigzag16x16[T Signed](data [][]T, result []T) {
	for i, c := range table16x16 {
		result[i] = data[c.R][c.C]
	}
}

func zigzag32x31[T Signed](data [][]T, result []T) {
	for i, c := range table32x31 {
		result[i] = data[c.R][c.C]
	}
}

func zigzag32x32[T Signed](data [][]T, result []T) {
	for i, c := range table32x32 {
		result[i] = data[c.R][c.C]
	}
}

func Zigzag[T Signed](data [][]T, rows, cols int) (result []T) {
	if rows != len(data) {
		return nil
	}
	if cols != len(data[0]) {
		return nil
	}
	result = make([]T, rows*cols)

	switch rows {
	case 8:
		if cols == 7 {
			zigzag8x7(data, result)
			return
		}
		zigzag8x8(data, result)
		return
	case 16:
		if cols == 15 {
			zigzag16x15(data, result)
			return
		}
		zigzag16x16(data, result)
		return
	case 32:
		if cols == 31 {
			zigzag32x31(data, result)
			return
		}
		zigzag32x32(data, result)
		return
	default:
		return nil
	}
}

func unzigzag8x7[T Signed](data []T, result [][]T) {
	for i, c := range table8x7 {
		result[c.R][c.C] = data[i]
	}
}

func unzigzag8x8[T Signed](data []T, result [][]T) {
	for i, c := range table8x8 {
		result[c.R][c.C] = data[i]
	}
}

func unzigzag16x15[T Signed](data []T, result [][]T) {
	for i, c := range table16x15 {
		result[c.R][c.C] = data[i]
	}
}

func unzigzag16x16[T Signed](data []T, result [][]T) {
	for i, c := range table16x16 {
		result[c.R][c.C] = data[i]
	}
}

func unzigzag32x31[T Signed](data []T, result [][]T) {
	for i, c := range table32x31 {
		result[c.R][c.C] = data[i]
	}
}

func unzigzag32x32[T Signed](data []T, result [][]T) {
	for i, c := range table32x32 {
		result[c.R][c.C] = data[i]
	}
}

func Unzigzag[T Signed](data []T, rows, cols int) (result [][]T) {
	result = make([][]T, rows)
	for i := 0; i < rows; i += 1 {
		result[i] = make([]T, cols)
	}

	switch rows {
	case 8:
		if cols == 7 {
			unzigzag8x7(data, result)
			return
		}
		unzigzag8x8(data, result)
		return
	case 16:
		if cols == 15 {
			unzigzag16x15(data, result)
			return
		}
		unzigzag16x16(data, result)
		return
	case 32:
		if cols == 31 {
			unzigzag32x31(data, result)
			return
		}
		unzigzag32x32(data, result)
		return
	default:
		return nil
	}
}
