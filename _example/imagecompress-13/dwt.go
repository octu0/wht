package main

// lift53 は LeGall 5/3 DWT のリフティング実装です。
func lift53(arr []int16) {
	n := len(arr)
	half := n / 2
	low := make([]int16, half)
	high := make([]int16, half)
	for i := 0; i < half; i += 1 {
		low[i] = arr[2*i]
		high[i] = arr[(2*i)+1]
	}
	for i := 0; i < half; i += 1 { // Predict
		l, r := int32(low[i]), int32(low[i])
		if i+1 < half {
			r = int32(low[i+1])
		}
		high[i] -= int16((l + r) >> 1)
	}
	for i := 0; i < half; i += 1 { // Update
		d, dp := int32(high[i]), int32(high[i])
		if 0 <= i-1 {
			dp = int32(high[i-1])
		}
		low[i] += int16((dp + d + 2) >> 2)
	}
	for i := 0; i < half; i += 1 {
		arr[i] = low[i]
		arr[half+i] = high[i]
	}
}

func invLift53(arr []int16) {
	n := len(arr)
	half := n / 2
	low := make([]int16, half)
	high := make([]int16, half)
	copy(low, arr[:half])
	copy(high, arr[half:])
	for i := 0; i < half; i += 1 { // Inv Update
		d, dp := int32(high[i]), int32(high[i])
		if 0 <= i-1 {
			dp = int32(high[i-1])
		}
		low[i] -= int16((dp + d + 2) >> 2)
	}
	for i := 0; i < half; i += 1 { // Inv Predict
		l, r := int32(low[i]), int32(low[i])
		if i+1 < half {
			r = int32(low[i+1])
		}
		high[i] += int16((l + r) >> 1)
	}
	for i := 0; i < half; i += 1 {
		arr[2*i] = low[i]
		arr[(2*i)+1] = high[i]
	}
}

// dwtBlock は 2次元の DWT を行います。
func dwtBlock(data [][]int16, size int) {
	buf := make([]int16, size)
	// Row transform
	for y := 0; y < size; y += 1 {
		lift53(data[y][:size])
	}
	// Col transform
	for x := 0; x < size; x += 1 {
		for y := 0; y < size; y += 1 {
			buf[y] = data[y][x]
		}
		lift53(buf)
		for y := 0; y < size; y++ {
			data[y][x] = buf[y]
		}
	}
}

// invDwtBlock は 2次元の IDWT を行います。
func invDwtBlock(data [][]int16, size int) {
	buf := make([]int16, size)
	// Inv Col transform
	for x := 0; x < size; x += 1 {
		for y := 0; y < size; y += 1 {
			buf[y] = data[y][x]
		}
		invLift53(buf)
		for y := 0; y < size; y++ {
			data[y][x] = buf[y]
		}
	}
	// Inv Row transform
	for y := 0; y < size; y += 1 {
		invLift53(data[y][:size])
	}
}

// dwtBlock2Level は 2層の 2次元 DWT を行います。
func dwtBlock2Level(data [][]int16, size int) {
	dwtBlock(data, size)
	if size < 16 {
		return
	}
	half := size / 2
	ll := make([][]int16, half)
	for y := 0; y < half; y += 1 {
		ll[y] = data[y][:half]
	}
	dwtBlock(ll, half)
}

// invDwtBlock2Level は 2層の 2次元 IDWT を行います。
func invDwtBlock2Level(data [][]int16, size int) {
	if 16 <= size {
		half := size / 2
		ll := make([][]int16, half)
		for y := 0; y < half; y += 1 {
			ll[y] = data[y][:half]
		}
		invDwtBlock(ll, half)
	}
	invDwtBlock(data, size)
}

// DWTPlane performs 2D DWT on a full image plane.
// width and height must be even.
func DWTPlane(data []int16, width, height int) {
	// Row transform
	for y := 0; y < height; y += 1 {
		row := data[y*width : (y+1)*width]
		lift53(row)
	}
	// Col transform
	col := make([]int16, height)
	for x := 0; x < width; x += 1 {
		for y := 0; y < height; y += 1 {
			col[y] = data[(y*width)+x]
		}
		lift53(col)
		for y := 0; y < height; y += 1 {
			data[(y*width)+x] = col[y]
		}
	}
}

// InvDWTPlane performs 2D IDWT on a full image plane.
func InvDWTPlane(data []int16, width, height int) {
	// Inv Col transform
	col := make([]int16, height)
	for x := 0; x < width; x += 1 {
		for y := 0; y < height; y += 1 {
			col[y] = data[(y*width)+x]
		}
		invLift53(col)
		for y := 0; y < height; y++ {
			data[(y*width)+x] = col[y]
		}
	}
	// Inv Row transform
	for y := 0; y < height; y += 1 {
		row := data[y*width : (y+1)*width]
		invLift53(row)
	}
}
