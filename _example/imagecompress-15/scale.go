package main

type RateController struct {
	maxbit             int
	totalProcessPixels int
	currentBits        int
	processedPixels    int
	baseShift          int
}

func (rc *RateController) CalcScale(addedBits int, addedPixels uint16) int {
	rc.currentBits += addedBits
	rc.processedPixels += int(addedPixels)

	// 処理済みピクセルに対する実際のBPP（bits per pixel）
	actualBPP := float64(rc.currentBits) / float64(rc.processedPixels)
	// 全体の目標BPP
	targetBPP := float64(rc.maxbit) / float64(rc.totalProcessPixels)

	// 実際/目標の比率に基づいて baseShift を調整
	ratio := actualBPP / targetBPP
	if ratio > 2.0 {
		rc.baseShift += 2
	} else if ratio > 1.2 {
		rc.baseShift += 1
	} else if ratio < 0.5 && rc.baseShift > 0 {
		rc.baseShift -= 2
	} else if ratio < 0.8 && rc.baseShift > 0 {
		rc.baseShift -= 1
	}

	if rc.baseShift < 0 {
		rc.baseShift = 0
	}
	if rc.baseShift > 8 {
		rc.baseShift = 8
	}

	return rc.baseShift
}

type rowFunc func(x, y uint16, size uint16, prediction int16) []int16

type scale struct {
	minVal, maxVal int16
	rowFn          rowFunc
}

func (s *scale) Rows(w, h uint16, size uint16, prediction int16, baseShift int) ([][]int16, int) {
	rows := make([][]int16, size)
	s.minVal = 32767
	s.maxVal = -32768
	for i := uint16(0); i < size; i += 1 {
		r := s.rowFn(w, h+i, size, prediction)
		rows[i] = r
		for _, v := range r {
			if v < s.minVal {
				s.minVal = v
			}
			if s.maxVal < v {
				s.maxVal = v
			}
		}
	}

	localScale := baseShift
	return rows, localScale
}

func newScale(rowFn rowFunc) *scale {
	return &scale{
		minVal: 32767,
		maxVal: -32768,
		rowFn:  rowFn,
	}
}
