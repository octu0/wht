# `wht`

[![MIT License](https://img.shields.io/github/license/octu0/wht)](https://github.com/octu0/wht/blob/master/LICENSE)
[![GoDoc](https://godoc.org/github.com/octu0/wht?status.svg)](https://godoc.org/github.com/octu0/wht)
[![Go Report Card](https://goreportcard.com/badge/github.com/octu0/wht)](https://goreportcard.com/report/github.com/octu0/wht)
[![Releases](https://img.shields.io/github/v/release/octu0/wht)](https://github.com/octu0/wht/releases)

`wht` is Walsh-Hadamard Transform and Zigzag scan implementation.

## Installation

```
$ go get github.com/octu0/wht
```

## Example

```go
package main

import (
	"fmt"

	"github.com/octu0/wht"
)

func main() {
	// Walsh-Hadamard Transform
	data := []int16{1, 0, 1, 0}
	wht.Transform(data)
	fmt.Println(data) // => [2 2 0 0]

	wht.Invert(data)
	fmt.Println(data) // => [1 0 1 0]

	// Zigzag scan
	matrix := [][]int16{
		{1, 2, 3, 4},
		{5, 6, 7, 8},
		{9, 10, 11, 12},
		{13, 14, 15, 16},
	}
	zigzag := wht.Zigzag(matrix)
	fmt.Println(zigzag) // => [1 2 5 9 6 3 4 7 10 13 14 11 8 12 15 16]

	orig := wht.Unzigzag(zigzag, 4)
	fmt.Println(orig) // => [[1 2 3 4] [5 6 7 8] [9 10 11 12] [13 14 15 16]]
}
```

# License

MIT, see LICENSE file for details.
