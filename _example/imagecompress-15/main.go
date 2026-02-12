package main

import (
	"bytes"
	"flag"
	"fmt"
	"time"

	_ "embed"
)

var (
	//go:embed src.png
	srcPng []byte
)

func main() {
	var bitrate int
	var benchmarkMode bool
	flag.IntVar(&bitrate, "bitrate", 100, "target bitrate in kbps")
	flag.BoolVar(&benchmarkMode, "benchmark", false, "run benchmark mode")
	flag.Parse()

	if benchmarkMode {
		runBenchmark(srcPng)
		return
	}

	ycbcr, err := pngToYCbCr(srcPng)
	if err != nil {
		panic(fmt.Sprintf("%+v", err))
	}

	srcbit := ycbcr.Bounds().Dx() * ycbcr.Bounds().Dy() * 8
	maxbit := bitrate * 1000
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

	layer0, layer1, layer2, err := decode(bytes.NewReader(out))
	if err != nil {
		panic(fmt.Sprintf("%+v", err))
	}

	// Decode Layer 0 (Thumbnail)
	if err := saveImage(layer0, "out_layer0.png"); err != nil {
		panic(fmt.Sprintf("%+v", err))
	}
	fmt.Printf("Layer0 decoded size: %v\n", layer0.Rect)

	// Decode Layer 0 + 1 (Medium)
	if err := saveImage(layer1, "out_layer1.png"); err != nil {
		panic(fmt.Sprintf("%+v", err))
	}
	fmt.Printf("Layer1 decoded size: %v\n", layer1.Rect)

	// Decode Layer 0 + 1 + 2 (High)
	if err := saveImage(layer2, "out_layer2.png"); err != nil {
		panic(fmt.Sprintf("%+v", err))
	}
	fmt.Printf("Layer2 decoded size: %v\n", layer2.Rect)
}
