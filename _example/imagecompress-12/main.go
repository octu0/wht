package main

import (
	"fmt"
	"time"

	_ "embed"
)

var (
	//go:embed src.png
	srcPng []byte
)

func main() {
	ycbcr, err := pngToYCbCr(srcPng)
	if err != nil {
		panic(fmt.Sprintf("%+v", err))
	}

	srcbit := ycbcr.Bounds().Dx() * ycbcr.Bounds().Dy() * 8
	maxbit := 500 * 1000 // Target total bits (approx)
	fmt.Printf("src %d bit\n", srcbit)
	fmt.Printf("target %d bit = %3.2f%%\n", maxbit, (float64(maxbit)/float64(srcbit))*100)

	t := time.Now()
	// encode returns layer0 (thumbnail), layer1 (medium diff), layer2 (high diff)
	layer0, layer1, layer2, err := encode(ycbcr, maxbit)
	if err != nil {
		panic(fmt.Sprintf("%+v", err))
	}
	elapsed := time.Since(t)

	original := len(ycbcr.Y) + len(ycbcr.Cb) + len(ycbcr.Cr)
	l0Size := len(layer0)
	l1Size := len(layer1)
	l2Size := len(layer2)
	totalSize := l0Size + l1Size + l2Size

	fmt.Printf(
		"elapse=%s\nLayer0: %3.2fKB\nLayer1: %3.2fKB\nLayer2: %3.2fKB\nTotal: %3.2fKB (Compressed %3.2f%%)\n",
		elapsed,
		float64(l0Size)/1024.0,
		float64(l1Size)/1024.0,
		float64(l2Size)/1024.0,
		float64(totalSize)/1024.0,
		(float64(totalSize)/float64(original))*100,
	)

	// Decode Layer 0 (Thumbnail)
	img0, err := decode(layer0)
	if err != nil {
		panic(fmt.Sprintf("Layer0 Decode: %+v", err))
	}
	if err := saveImage(img0, "out_layer0.png"); err != nil {
		panic(fmt.Sprintf("%+v", err))
	}
	fmt.Printf("Layer0 decoded size: %v\n", img0.Rect)

	// Decode Layer 0 + 1 (Medium)
	img1, err := decode(layer0, layer1)
	if err != nil {
		panic(fmt.Sprintf("Layer1 Decode: %+v", err))
	}
	if err := saveImage(img1, "out_layer1.png"); err != nil {
		panic(fmt.Sprintf("%+v", err))
	}
	fmt.Printf("Layer1 decoded size: %v\n", img1.Rect)

	// Decode Layer 0 + 1 + 2 (High)
	img2, err := decode(layer0, layer1, layer2)
	if err != nil {
		panic(fmt.Sprintf("Layer2 Decode: %+v", err))
	}
	if err := saveImage(img2, "out_layer2.png"); err != nil {
		panic(fmt.Sprintf("%+v", err))
	}
	fmt.Printf("Layer2 decoded size: %v\n", img2.Rect)
}
