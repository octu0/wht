package main

import (
	"image"
	"image/color"
	"math"
)

// CalcPSNR calculates Peak Signal-to-Noise Ratio for Y, Cb, Cr, and Average.
func CalcPSNR(img1, img2 image.Image) (float64, float64, float64, float64) {
	bounds := img1.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	var mseY, mseCb, mseCr float64

	for y := 0; y < h; y += 1 {
		for x := 0; x < w; x += 1 {
			r1, g1, b1, _ := img1.At(x, y).RGBA()
			r2, g2, b2, _ := img2.At(x, y).RGBA()

			y1, cb1, cr1 := color.RGBToYCbCr(uint8(r1>>8), uint8(g1>>8), uint8(b1>>8))
			y2, cb2, cr2 := color.RGBToYCbCr(uint8(r2>>8), uint8(g2>>8), uint8(b2>>8))

			dy := float64(y1) - float64(y2)
			dcb := float64(cb1) - float64(cb2)
			dcr := float64(cr1) - float64(cr2)

			mseY += (dy * dy)
			mseCb += (dcb * dcb)
			mseCr += (dcr * dcr)
		}
	}

	pixels := float64(w * h)
	mseY /= pixels
	mseCb /= pixels
	mseCr /= pixels

	psnrY := 20 * math.Log10(255.0/math.Sqrt(mseY))
	psnrCb := 20 * math.Log10(255.0/math.Sqrt(mseCb))
	psnrCr := 20 * math.Log10(255.0/math.Sqrt(mseCr))

	if mseY == 0 {
		psnrY = 100.0 // Infinite
	}
	if mseCb == 0 {
		psnrCb = 100.0
	}
	if mseCr == 0 {
		psnrCr = 100.0
	}

	avg := (psnrY + psnrCb + psnrCr) / 3.0
	return psnrY, psnrCb, psnrCr, avg
}

// CalcSSIM calculates Structural Similarity Index for Y, Cb, Cr, and Average.
func CalcSSIM(img1, img2 image.Image) (float64, float64, float64, float64) {
	// Standard SSIM constants
	c1 := (0.01 * 0.01) * (255 * 255)
	c2 := (0.03 * 0.03) * (255 * 255)

	// Using a simple 8x8 block approach for simplified SSIM or sliding window.
	// For "exact" SSIM we typically use a Gaussian window, but a block average is a common approximation for simple codec comparison.
	// However, to be more robust, let's implement a basic sliding window or block-based approach.
	// Given the constraints and typical Go implementations for simple checks, block-based is faster and often sufficient.
	// Let's use 8x8 blocks.

	y1Plane, cb1Plane, cr1Plane := extractPlanes(img1)
	y2Plane, cb2Plane, cr2Plane := extractPlanes(img2)

	ssimY := ssimPlane(y1Plane, y2Plane, c1, c2)
	ssimCb := ssimPlane(cb1Plane, cb2Plane, c1, c2)
	ssimCr := ssimPlane(cr1Plane, cr2Plane, c1, c2)

	return ssimY, ssimCb, ssimCr, (ssimY + ssimCb + ssimCr) / 3.0
}

func extractPlanes(img image.Image) ([][]float64, [][]float64, [][]float64) {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	yPlane := make([][]float64, h)
	cbPlane := make([][]float64, h)
	crPlane := make([][]float64, h)

	for y := 0; y < h; y += 1 {
		yPlane[y] = make([]float64, w)
		cbPlane[y] = make([]float64, w)
		crPlane[y] = make([]float64, w)
		for x := 0; x < w; x += 1 {
			r, g, b, _ := img.At(x, y).RGBA()
			yy, cb, cr := color.RGBToYCbCr(uint8(r>>8), uint8(g>>8), uint8(b>>8))
			yPlane[y][x] = float64(yy)
			cbPlane[y][x] = float64(cb)
			crPlane[y][x] = float64(cr)
		}
	}
	return yPlane, cbPlane, crPlane
}

func ssimPlane(p1, p2 [][]float64, c1, c2 float64) float64 {
	h := len(p1)
	w := len(p1[0])

	// Mean SSIM over all 8x8 blocks (non-overlapping for speed, or overlapping?)
	// Standard SSIM is sliding window (1 px step). That's very slow in pure Go without optimization.
	// Let's do non-overlapping 8x8 blocks for speed in this context, or maybe 4x4.
	// Actually, let's try to do it properly with a small window if efficient enough, or blocks.
	// Given this is a codec comparison tool, block-based (like 8x8 DCT blocks) is often used as a proxy,
	// but the user might want standard SSIM.
	// Let's stick to non-overlapping 8x8 for now to keep performance reasonable for large images in pure Go.

	blockSize := 8
	totalSSIM := 0.0
	count := 0

	for y := 0; y < h-blockSize; y += blockSize {
		for x := 0; x < w-blockSize; x += blockSize {
			totalSSIM += ssimBlock(p1, p2, x, y, blockSize, c1, c2)
			count += 1
		}
	}

	if count == 0 {
		return 0
	}
	return totalSSIM / float64(count)
}

// CalcMSSSIM calculates Multi-Scale Structural Similarity Index for Y, Cb, Cr, and Average.
func CalcMSSSIM(img1, img2 image.Image) (float64, float64, float64, float64) {
	// Constants for MS-SSIM
	// Weights for 5 scales: 0.0448, 0.2856, 0.3001, 0.2363, 0.1333
	weights := []float64{0.0448, 0.2856, 0.3001, 0.2363, 0.1333}
	levels := len(weights)

	// Standard constants K1, K2. C1 = (K1*L)^2, C2 = (K2*L)^2
	// L=255
	c1 := (0.01 * 0.01) * (255 * 255)
	c2 := (0.03 * 0.03) * (255 * 255)

	y1Plane, cb1Plane, cr1Plane := extractPlanes(img1)
	y2Plane, cb2Plane, cr2Plane := extractPlanes(img2)

	msssimY := msssimPlane(y1Plane, y2Plane, weights, levels, c1, c2)
	msssimCb := msssimPlane(cb1Plane, cb2Plane, weights, levels, c1, c2)
	msssimCr := msssimPlane(cr1Plane, cr2Plane, weights, levels, c1, c2)

	return msssimY, msssimCb, msssimCr, (msssimY + msssimCb + msssimCr) / 3.0
}

func msssimPlane(p1, p2 [][]float64, weights []float64, levels int, c1, c2 float64) float64 {
	// mcsSum := 0.0
	// We accumulate exponentiated values: Product(val^weight).
	// To be safe with float precision, usually done as exp(sum(weight * log(val))).
	// But direct product is fine for small count.

	finalScore := 1.0

	currP1 := p1
	currP2 := p2

	for i := 0; i < levels; i += 1 {
		// Calculate Luminance (l) and Contrast*Structure (cs) for this scale
		l, cs := ssimComponents(currP1, currP2, c1, c2)

		if i < levels-1 {
			// For scales 1 to M-1, we use only CS
			if 0 < cs {
				finalScore *= math.Pow(cs, weights[i])
			}
		} else {
			// For scale M, we use both L and CS
			if 0 < l {
				finalScore *= math.Pow(l, weights[i])
			}
			if 0 < cs {
				finalScore *= math.Pow(cs, weights[i])
			}
		}

		// Downsample for next iteration
		if i < levels-1 {
			currP1 = downsamplePlane(currP1)
			currP2 = downsamplePlane(currP2)
		}
	}
	return finalScore
}

// downsamplePlane reduces image size by 2x using simple 2x2 averaging (box filter).
func downsamplePlane(p [][]float64) [][]float64 {
	h := len(p)
	w := len(p[0])
	newH := h / 2
	newW := w / 2

	newP := make([][]float64, newH)
	for y := 0; y < newH; y += 1 {
		newP[y] = make([]float64, newW)
		for x := 0; x < newW; x += 1 {
			// Average of 2x2 block
			sum := p[y*2][x*2] + p[y*2][x*2+1] + p[y*2+1][x*2] + p[y*2+1][x*2+1]
			newP[y][x] = sum / 4.0
		}
	}
	return newP
}

// ssimComponents calculates average L and CS terms over the image.
func ssimComponents(p1, p2 [][]float64, c1, c2 float64) (float64, float64) {
	h := len(p1)
	w := len(p1[0])
	blockSize := 8

	totalL := 0.0
	totalCS := 0.0
	count := 0

	// Reuse ssimBlock logic but split L and CS
	for y := 0; y < h-blockSize; y += blockSize {
		for x := 0; x < w-blockSize; x += blockSize {
			l, cs := ssimBlockComponents(p1, p2, x, y, blockSize, c1, c2)
			totalL += l
			totalCS += cs
			count += 1
		}
	}

	if count == 0 {
		return 0, 0
	}
	return totalL / float64(count), totalCS / float64(count)
}

func ssimBlockComponents(p1, p2 [][]float64, x, y, size int, c1, c2 float64) (float64, float64) {
	var mu1, mu2 float64
	for j := 0; j < size; j += 1 {
		for i := 0; i < size; i += 1 {
			mu1 += p1[y+j][x+i]
			mu2 += p2[y+j][x+i]
		}
	}
	n := float64(size * size)
	mu1 /= n
	mu2 /= n

	var sigma1Sq, sigma2Sq, sigma12 float64
	for j := 0; j < size; j += 1 {
		for i := 0; i < size; i += 1 {
			d1 := p1[y+j][x+i] - mu1
			d2 := p2[y+j][x+i] - mu2
			sigma1Sq += (d1 * d1)
			sigma2Sq += (d2 * d2)
			sigma12 += (d1 * d2)
		}
	}
	sigma1Sq /= (n - 1)
	sigma2Sq /= (n - 1)
	sigma12 /= (n - 1)

	// L term = (2*mu1*mu2 + C1) / (mu1^2 + mu2^2 + C1)
	// CS term = (2*sigma12 + C2) / (sigma1^2 + sigma2^2 + C2)

	lNum := (2 * mu1 * mu2) + c1
	lDen := (mu1 * mu1) + (mu2 * mu2) + c1

	csNum := (2 * sigma12) + c2
	csDen := sigma1Sq + sigma2Sq + c2

	return lNum / lDen, csNum / csDen
}

func ssimBlock(p1, p2 [][]float64, x, y, size int, c1, c2 float64) float64 {
	l, cs := ssimBlockComponents(p1, p2, x, y, size, c1, c2)
	return l * cs
}
