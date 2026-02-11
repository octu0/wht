package main

// RateController は hoge.go の設計に基づき、仮想バッファを用いて
// ビットレートを制御し、適切な量子化シフト量(baseShift)を決定します。
type RateController struct {
	maxbit             int
	totalProcessPixels int
	currentBits        int
	processedPixels    int
	baseShift          int
}

func (rc *RateController) CalcScale(addedBits, addedPixels int) int {
	rc.currentBits += addedBits
	rc.processedPixels += addedPixels

	// 現在の進捗における理想的な消費ビット数
	targetBitsProgress := int(float64(rc.maxbit) * (float64(rc.processedPixels) / float64(rc.totalProcessPixels)))

	// 目標からの乖離 (Virtual Buffer)
	diff := rc.currentBits - targetBitsProgress
	threshold := rc.maxbit / 10

	// 乖離に応じて baseShift を段階的に変更
	if diff > threshold {
		rc.baseShift++
		// 調整をマイルドにするため、仮想バッファの一部を解消したことにする
		rc.currentBits -= threshold / 2
	} else if diff < -threshold {
		if rc.baseShift > 0 {
			rc.baseShift--
			rc.currentBits += threshold / 2
		}
	}

	if rc.baseShift < 0 {
		rc.baseShift = 0
	}
	if rc.baseShift > 8 {
		rc.baseShift = 8
	}

	return rc.baseShift
}

func newRateController(maxbit int, width, height uint16) *RateController {
	totalPixels := width * height
	// Y + Cb + Cr の合計ピクセル数で進捗を管理
	totalProcessPixels := int(totalPixels + (totalPixels / 2))

	return &RateController{
		maxbit:             maxbit,
		totalProcessPixels: totalProcessPixels,
		currentBits:        0,
		processedPixels:    0,
		baseShift:          2, // 初期値を引き上げ
	}
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
