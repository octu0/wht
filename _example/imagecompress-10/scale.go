package main

type Scaler struct {
	maxbit             int
	totalProcessPixels int
	currentBits        int
	processedPixels    int
}

func (s *Scaler) CalcScale(addedBits, addedPixels int) int {
	s.currentBits += addedBits
	s.processedPixels += addedPixels

	// target bits for current progress
	targetBits := int(float64(s.maxbit) * (float64(s.processedPixels) / float64(s.totalProcessPixels)))
	diff := s.currentBits - targetBits

	if diff <= 0 {
		return 1
	}
	// Simple P-control
	// if diff is 10% of maxbit, scale up significantly
	// 500kbps = 62500 bytes. 10% = 6250 bytes.
	// scale = 1 + diff / (maxbit / 50) roughly
	scale := 1 + int(diff/(s.maxbit/50))
	scale = min(16, scale)
	return scale
}

func newScaler(maxbit int, width, height int) *Scaler {
	currentBits := 0
	processedPixels := 0
	totalPixels := width * height
	// Y: total pixels, Cb: 1/4, Cr: 1/4 -> Total logical pixels for tracking progress = 1.5 * pixels?
	// To simplify, just track pixel count of Y plane + Cb + Cr
	totalProcessPixels := totalPixels + (totalPixels / 2) // Y + Cb + Cr

	return &Scaler{
		maxbit:             maxbit,
		totalProcessPixels: totalProcessPixels,
		currentBits:        currentBits,
		processedPixels:    processedPixels,
	}
}
