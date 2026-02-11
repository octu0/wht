package main

import (
	"bytes"
	"fmt"
	"time"

	_ "embed"
)

var (
	//go:embed src.png
	srcPng []byte
)

func printSubband(data [][]int16) {
	for _, row := range data {
		for _, val := range row {
			fmt.Printf("%3d ", val)
		}
		fmt.Println()
	}
}

func main() {
	ycbcr, err := pngToYCbCr(srcPng)
	if err != nil {
		panic(fmt.Sprintf("%+v", err))
	}

	srcbit := ycbcr.Bounds().Dx() * ycbcr.Bounds().Dy() * 8
	maxbit := 100 * 1000
	fmt.Printf("src %d bit\n", srcbit)
	fmt.Printf("target %d bit = %3.2f%%\n", maxbit, (float64(maxbit)/float64(srcbit))*100)

	t := time.Now()
	out, err := encode(ycbcr, maxbit)
	if err != nil {
		panic(fmt.Sprintf("%+v", err))
	}

	original := len(ycbcr.Y) + len(ycbcr.Cb) + len(ycbcr.Cr)
	compressedSize := len(out)

	fmt.Printf(
		"elapse=%s %3.2fKB -> %3.2fKB compressed %3.2f%%\n",
		time.Since(t),
		float64(original)/1024.0,
		float64(compressedSize)/1024.0,
		(float64(compressedSize)/float64(original))*100,
	)

	outData, err := decode(bytes.NewReader(out))
	if err != nil {
		panic(fmt.Sprintf("%+v", err))
	}

	if err := saveImage(ycbcr, "out_origin.png"); err != nil {
		panic(fmt.Sprintf("%+v", err))
	}
	if err := saveImage(outData, "out_new.png"); err != nil {
		panic(fmt.Sprintf("%+v", err))
	}
}
