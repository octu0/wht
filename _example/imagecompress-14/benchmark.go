package main

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
)

// BenchmarkMetrics holds the calculated quality metrics for a single image layer.
type BenchmarkMetrics struct {
	PSNR   float64
	PSNR_Y float64
	PSNR_C float64 // Cb
	PSNR_K float64 // Cr (use K to avoid confusion with C? Cb is Blue-difference, Cr is Red-difference. Let's use Y/Cb/Cr naming)

	SSIM   float64
	MSSSIM float64

	// Detail breakdown
	Y, Cb, Cr float64
}

// BenchmarkResult holds the metrics for all three layers (Large, Mid, Small).
type BenchmarkResult struct {
	Large  BenchmarkMetrics
	Mid    BenchmarkMetrics
	Small  BenchmarkMetrics
	SizeKB float64
}

func runBenchmark(src []byte) {
	ycbcr, err := pngToYCbCr(src)
	if err != nil {
		panic(fmt.Sprintf("%+v", err))
	}

	// Convert to 4:2:0 for fairness (codec uses 4:2:0) and for ResizeHalf correctness
	originImg := image.NewYCbCr(ycbcr.Bounds(), image.YCbCrSubsampleRatio420)
	if err := convertYCbCr(originImg, ycbcr); err != nil {
		panic(err)
	}

	// Prepare Reference Images for Multi-Resolution
	// Use explicit initialization with :=
	refLarge := originImg
	refMid := resizeHalfNN(refLarge)
	refSmall := resizeHalfNN(refMid)

	// Save reference images for verification (Explicit error handling)
	if err := saveImage(refLarge, "out_origin_large.png"); err != nil {
		panic(err)
	}
	if err := saveImage(refMid, "out_origin_mid.png"); err != nil {
		panic(err)
	}
	if err := saveImage(refSmall, "out_origin_small.png"); err != nil {
		panic(err)
	}

	fmt.Println("=== JPEG Comparison ===")
	// Quality 50 to 100
	for q := 50; q <= 100; q += 10 {
		runJPEGComparison(q, originImg, refMid, refSmall)
	}

	fmt.Println("\n=== Custom Codec Comparison ===")
	// Bitrate 100k to 2000k
	for bitrate := 100; bitrate <= 2000; bitrate += 300 {
		runCustomCodecComparison(bitrate, originImg, refMid, refSmall)
	}
}

func runJPEGComparison(q int, refLarge, refMid, refSmall *image.YCbCr) {
	// Encode to JPEG
	buf := bytes.Buffer{}
	opts := &jpeg.Options{Quality: q}

	// Explicit interface conversion for jpeg.Encode
	var iRefLarge image.Image = refLarge

	if err := jpeg.Encode(&buf, iRefLarge, opts); err != nil {
		panic(err)
	}

	encodedBytes := buf.Bytes()
	sizeKB := float64(len(encodedBytes)) / 1024.0

	// Decode back (Full size)
	decodedJpeg, err := jpeg.Decode(bytes.NewReader(encodedBytes))
	if err != nil {
		panic(err)
	}

	// Create JPEG Multi-Resolution views (Downsample)
	// We need to verify if decodedJpeg is YCbCr.
	// Use Type assertion with explicit failure handling.
	jpgLarge, ok := decodedJpeg.(*image.YCbCr)
	if ok != true {
		// Fallback: convert to YCbCr
		b := decodedJpeg.Bounds()
		jpgLarge = image.NewYCbCr(b, image.YCbCrSubsampleRatio420)
		convertYCbCr(jpgLarge, decodedJpeg)
	}

	jpgMid := resizeHalfNN(jpgLarge)
	jpgSmall := resizeHalfNN(jpgMid)

	// Calculate and Print Metrics (Pass explicit images)
	// Interface wrapper for helper function
	var iJpgLarge image.Image = jpgLarge
	var iJpgMid image.Image = jpgMid
	var iJpgSmall image.Image = jpgSmall

	printMetrics("JPEG Q", q, sizeKB,
		calcMetrics(refLarge, iJpgLarge),
		calcMetrics(refMid, iJpgMid),
		calcMetrics(refSmall, iJpgSmall),
	)

	// Save output for inspection
	saveImage(jpgLarge, fmt.Sprintf("out_q%d_L.png", q))
	saveImage(jpgMid, fmt.Sprintf("out_q%d_M.png", q))
	saveImage(jpgSmall, fmt.Sprintf("out_q%d_S.png", q))
}

func runCustomCodecComparison(bitrate int, originImg, refMid, refSmall *image.YCbCr) {
	targetBits := bitrate * 1000

	// Encode
	layer0, layer1, layer2, err := encode(originImg, targetBits)
	if err != nil {
		panic(err)
	}

	totalSize := len(layer0) + len(layer1) + len(layer2)
	sizeKB := float64(totalSize) / 1024.0

	// Decode Multi-Resolution

	// Small (Layer0)
	decSmall, err := decode(layer0)
	if err != nil {
		panic(err)
	}

	// Mid (Layer0+1)
	decMid, err := decode(layer0, layer1)
	if err != nil {
		panic(err)
	}

	// Large (Layer0+1+2)
	decLarge, err := decode(layer0, layer1, layer2)
	if err != nil {
		panic(err)
	}

	// Calculate Metrics
	// The decode function returns *image.YCbCr, which implements image.Image.
	// But calcMetrics expects image.Image and *image.YCbCr?
	// Let's make calcMetrics take two image.Image to be safe, or explicit.
	// Ref is *image.YCbCr. Target is *image.YCbCr (from decode).

	// Helper interface conversion
	var iRefLarge image.Image = originImg
	var iRefMid image.Image = refMid
	var iRefSmall image.Image = refSmall

	// dec* are already *image.YCbCr, which is image.Image.
	// But valid to pass directly.

	printMetrics("MY   Rate", bitrate, sizeKB,
		calcMetrics(iRefLarge, decLarge),
		calcMetrics(iRefMid, decMid),
		calcMetrics(iRefSmall, decSmall),
	)

	// Save output
	saveImage(decLarge, fmt.Sprintf("out_n%d_L.png", bitrate))
	saveImage(decMid, fmt.Sprintf("out_n%d_M.png", bitrate))
	saveImage(decSmall, fmt.Sprintf("out_n%d_S.png", bitrate))
}

func calcMetrics(ref, target image.Image) BenchmarkMetrics {
	// Calculate PSNR
	y, cb, cr, avg := CalcPSNR(ref, target)

	// Calculate SSIM
	_, _, _, ssim := CalcSSIM(ref, target)

	// Calculate MS-SSIM
	_, _, _, msssim := CalcMSSSIM(ref, target)

	return BenchmarkMetrics{
		PSNR:   avg,
		PSNR_Y: y,
		PSNR_C: cb,
		PSNR_K: cr, // Maps to Cr
		SSIM:   ssim,
		MSSSIM: msssim,

		// Detail breakdown
		Y:  y,
		Cb: cb,
		Cr: cr,
	}
}

func resizeHalfNN(src *image.YCbCr) *image.YCbCr {
	w, h := src.Rect.Dx(), src.Rect.Dy()
	dstW, dstH := w/2, h/2
	dst := image.NewYCbCr(image.Rect(0, 0, dstW, dstH), image.YCbCrSubsampleRatio420)

	// Resize Y (Nearest Neighbor)
	for y := 0; y < dstH; y += 1 {
		for x := 0; x < dstW; x += 1 {
			sy, sx := y*2, x*2
			dst.Y[dst.YOffset(x, y)] = src.Y[src.YOffset(sx, sy)]
		}
	}

	// Resize Cb, Cr (Nearest Neighbor)
	dstCW, dstCH := dstW/2, dstH/2

	for y := 0; y < dstCH; y += 1 {
		for x := 0; x < dstCW; x += 1 {
			// Convert Chroma coordinates to Image coordinates for COffset
			// Dest Image Coord: (x*2, y*2)
			// Source Image Coord: (x*4, y*4)

			dOff := dst.COffset(x*2, y*2)
			sOff := src.COffset(x*4, y*4)

			dst.Cb[dOff] = src.Cb[sOff]
			dst.Cr[dOff] = src.Cr[sOff]
		}
	}

	return dst
}

func printMetrics(prefix string, val int, sizeKB float64, l, m, s BenchmarkMetrics) {
	if prefix == "JPEG Q" {
		fmt.Printf("%s=%3d Size=%6.2fKB\n", prefix, val, sizeKB)
	} else {
		// For Bitrate (k unit)
		fmt.Printf("%s=%4d k Size=%6.2fKB\n", prefix, val, sizeKB)
	}

	printLayerMetric("L", l)
	printLayerMetric("M", m)
	printLayerMetric("S", s)
}

func printLayerMetric(label string, m BenchmarkMetrics) {
	// Explicit format string
	fmt.Printf(
		" [%s] PSNR=%.2f SSIM=%.4f MS-SSIM=%.4f (Y:%.2f Cb:%.2f Cr:%.2f)\n",
		label,
		m.PSNR,
		m.SSIM,
		m.MSSSIM,
		m.Y,
		m.Cb,
		m.Cr,
	)
}
