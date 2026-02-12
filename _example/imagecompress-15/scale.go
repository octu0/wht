package main

import (
	"math"
)

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
	targetBitsProgress := float64(rc.maxbit) * (float64(rc.processedPixels) / float64(rc.totalProcessPixels))
	if 0 < targetBitsProgress {
		overshoot := float64(rc.currentBits) / targetBitsProgress
		switch {
		case 2.0 < overshoot:
			rc.baseShift += 2
		case 1.3 < overshoot:
			rc.baseShift += 1
		case overshoot < 0.5 && 0 < rc.baseShift:
			rc.baseShift -= 2
		case overshoot < 0.8 && 0 < rc.baseShift:
			rc.baseShift -= 1
		}
	}

	if rc.baseShift < 0 {
		rc.baseShift = 0
	}
	if 5 < rc.baseShift {
		rc.baseShift = 5
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
	s.minVal = math.MaxInt16
	s.maxVal = math.MinInt16
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
		minVal: math.MaxInt16,
		maxVal: math.MinInt16,
		rowFn:  rowFn,
	}
}
