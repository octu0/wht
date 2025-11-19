package wht_test

import (
	"fmt"

	"github.com/octu0/wht"
)

func Example() {
	data := []int16{1, 0, 1, 0}
	wht.Transform(data)
	fmt.Println(data)

	wht.Invert(data)
	fmt.Println(data)

	// Output:
	// [2 0 0 2]
	// [1 0 1 0]
}

func ExampleTransform4() {
	x := wht.Transform4([4]int16{1, 2, 3, 4})
	fmt.Println(x)
	y := wht.Invert4(x)
	fmt.Println(y)

	// Output:
	// [10 -4 0 -2]
	// [1 2 3 4]
}

func ExampleZigzag() {
	matrix := [][]int16{
		{1, 2, 3, 4},
		{5, 6, 7, 8},
		{9, 10, 11, 12},
		{13, 14, 15, 16},
	}
	zigzag := wht.Zigzag(matrix)
	fmt.Println(zigzag)

	orig := wht.Unzigzag(zigzag, 4)
	fmt.Println(orig)

	// Output:
	// [1 2 5 9 6 3 4 7 10 13 14 11 8 12 15 16]
	// [[1 2 3 4] [5 6 7 8] [9 10 11 12] [13 14 15 16]]
}
