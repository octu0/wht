package main

type Signed interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64
}

type coord struct {
	R, C uint8
}

func Zigzag[T Signed](data [][]T, size int) []T {
	result := make([]T, size*size)
	switch size {
	case 4:
		for i, c := range table4x4 {
			result[i] = data[c.R][c.C]
		}
	case 8:
		for i, c := range table8x8 {
			result[i] = data[c.R][c.C]
		}
	case 12:
		for i, c := range table12x12 {
			result[i] = data[c.R][c.C]
		}
	case 16:
		for i, c := range table16x16 {
			result[i] = data[c.R][c.C]
		}
	case 24:
		for i, c := range table24x24 {
			result[i] = data[c.R][c.C]
		}
	case 32:
		for i, c := range table32x32 {
			result[i] = data[c.R][c.C]
		}
	default:
		return nil
	}
	return result
}

func Unzigzag[T Signed](data []T, size int) [][]T {
	result := make([][]T, size)
	for i := 0; i < size; i++ {
		result[i] = make([]T, size)
	}
	switch size {
	case 4:
		for i, c := range table4x4 {
			result[c.R][c.C] = data[i]
		}
	case 8:
		for i, c := range table8x8 {
			result[c.R][c.C] = data[i]
		}
	case 12:
		for i, c := range table12x12 {
			result[c.R][c.C] = data[i]
		}
	case 16:
		for i, c := range table16x16 {
			result[c.R][c.C] = data[i]
		}
	case 24:
		for i, c := range table24x24 {
			result[c.R][c.C] = data[i]
		}
	case 32:
		for i, c := range table32x32 {
			result[c.R][c.C] = data[i]
		}
	default:
		return nil
	}
	return result
}
