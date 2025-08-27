package wht

import "fmt"

func Example() {
	data := []int16{1, 0, 1, 0}
	Transform(data)
	fmt.Println(data)

	Invert(data)
	fmt.Println(data)

	x := Transform4([4]int16{1, 2, 3, 4})
	fmt.Println(x)
	y := Invert4(x)
	fmt.Println(y)

	matrix := []int16{
		1, 2, 3, 4,
		5, 6, 7, 8,
		9, 10, 11, 12,
		13, 14, 15, 16,
	}
	zigzag := Zigzag(matrix, 4)
	fmt.Println(zigzag)

	orig := Unzigzag(zigzag, 4)
	fmt.Println(orig)

	// Output:
	// [2 2 0 0]
	// [1 0 1 0]
	// [10 -4 0 -2]
	// [1 2 3 4]
	// [1 2 5 9 6 3 4 7 10 13 14 11 8 12 15 16]
	// [1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16]
}
