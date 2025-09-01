package wht

func Zigzag[T Signed](matrix [][]T) []T {
	n := len(matrix)
	maxN := n * n

	result := make([]T, maxN)
	row, col := 0, 0
	goingUp := true

	for i := 0; i < maxN; i += 1 {
		result[i] = matrix[row][col]

		cursor(n, &row, &col, &goingUp)
	}
	return result
}

func Unzigzag[T Signed](data []T, stride int) [][]T {
	if len(data) != stride*stride {
		return nil
	}
	maxN := stride * stride

	result := make([][]T, stride)
	row, col := 0, 0
	goingUp := true

	for i := 0; i < stride; i += 1 {
		result[i] = make([]T, stride)
	}
	for i := 0; i < maxN; i += 1 {
		result[row][col] = data[i]

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
