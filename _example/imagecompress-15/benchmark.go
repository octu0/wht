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
	PSNR_K float64 // Cr

	SSIM   float64
	MSSSIM float64

	// Detail breakdown
	Y, Cb, Cr float64
}

func runBenchmark(src []byte) {
	ycbcr, err := pngToYCbCr(src)
	if err != nil {
		panic(fmt.Sprintf("%+v", err))
	}

	// Convert to 4:2:0 for fairness (codec uses 4:2:0)
	originImg := image.NewYCbCr(ycbcr.Bounds(), image.YCbCrSubsampleRatio420)
	if err := convertYCbCr(originImg, ycbcr); err != nil {
		panic(err)
	}

	// Prepare Reference Images for Multi-Resolution
	refLarge := originImg
	refMid := resizeHalfNN(refLarge)
	refSmall := resizeHalfNN(refMid)

	fmt.Printf("Reference: Large=%v Mid=%v Small=%v\n", refLarge.Rect, refMid.Rect, refSmall.Rect)

	fmt.Println("\n=== JPEG Comparison ===")
	for q := 50; q <= 100; q += 10 {
		runJPEGComparison(q, refLarge, refMid, refSmall)
	}

	fmt.Println("\n=== Custom Codec Comparison ===")
	for bitrate := 100; bitrate <= 500; bitrate += 100 {
		runCustomCodecComparison(bitrate, originImg, refMid, refSmall)
	}
}

func runJPEGComparison(q int, refLarge, refMid, refSmall *image.YCbCr) {
	// Encode to JPEG
	buf := bytes.Buffer{}
	opts := &jpeg.Options{Quality: q}

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

	// Convert to YCbCr
	jpgLarge, ok := decodedJpeg.(*image.YCbCr)
	if ok != true {
		b := decodedJpeg.Bounds()
		jpgLarge = image.NewYCbCr(b, image.YCbCrSubsampleRatio420)
		convertYCbCr(jpgLarge, decodedJpeg)
	}

	jpgMid := resizeHalfNN(jpgLarge)
	jpgSmall := resizeHalfNN(jpgMid)

	var iJpgLarge image.Image = jpgLarge
	var iJpgMid image.Image = jpgMid
	var iJpgSmall image.Image = jpgSmall

	printMetrics("JPEG Q", q, sizeKB,
		calcMetrics(refLarge, iJpgLarge),
		calcMetrics(refMid, iJpgMid),
		calcMetrics(refSmall, iJpgSmall),
	)
}

func runCustomCodecComparison(bitrate int, originImg, refMid, refSmall *image.YCbCr) {
	targetBits := bitrate * 1000

	// Encode
	out, err := encode(originImg, targetBits)
	if err != nil {
		panic(err)
	}

	sizeKB := float64(len(out)) / 1024.0

	// Decode Multi-Resolution
	decSmall, decMid, decLarge, err := decode(bytes.NewReader(out))
	if err != nil {
		panic(err)
	}

	var iRefLarge image.Image = originImg
	var iRefMid image.Image = refMid
	var iRefSmall image.Image = refSmall

	printMetrics("MY   Rate", bitrate, sizeKB,
		calcMetrics(iRefLarge, decLarge),
		calcMetrics(iRefMid, decMid),
		calcMetrics(iRefSmall, decSmall),
	)
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
		PSNR_K: cr,
		SSIM:   ssim,
		MSSSIM: msssim,

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
		fmt.Printf("%s=%4d k Size=%6.2fKB\n", prefix, val, sizeKB)
	}

	printLayerMetric("L", l)
	printLayerMetric("M", m)
	printLayerMetric("S", s)
}

func printLayerMetric(label string, m BenchmarkMetrics) {
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
