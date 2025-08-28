package wht

func Zigzag[T Signed](data []T, stride int) []T {
	if stride < 1 {
		return nil
	}
	if len(data) != stride*stride {
		return nil
	}
	maxN := stride * stride

	result := make([]T, maxN)
	row, col := 0, 0
	goingUp := true

	for i := 0; i < maxN; i += 1 {
		index := row*stride + col
		result[i] = data[index]

		cursor(stride, &row, &col, &goingUp)
	}
	return result
}

func Unzigzag[T Signed](data []T, stride int) []T {
	if stride < 1 {
		return nil
	}
	if len(data) != stride*stride {
		return nil
	}
	maxN := stride * stride

	result := make([]T, maxN)
	row, col := 0, 0
	goingUp := true

	for i := 0; i < maxN; i += 1 {
		index := row*stride + col
		result[index] = data[i]

		cursor(stride, &row, &col, &goingUp)
	}
	return result
}

func cursor(n int, row, col *int, up *bool) {
	if *up {
		switch {
		case *col == n-1:
			*row += 1
			*up = false
		case *row == 0:
			*col += 1
			*up = false
		default:
			*row -= 1
			*col += 1
		}
	} else {
		switch {
		case *row == n-1:
			*col += 1
			*up = true
		case *col == 0:
			*row += 1
			*up = true
		default:
			*row += 1
			*col -= 1
		}
	}
}
