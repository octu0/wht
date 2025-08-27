package wht

func Transform4(in [4]int16) [4]int16 {
	return [4]int16{
		(in[0] + in[1] + in[2] + in[3]),
		(in[0] + in[1] - in[2] - in[3]),
		(in[0] - in[1] - in[2] + in[3]),
		(in[0] - in[1] + in[2] - in[3]),
	}
}

func Invert4(in [4]int16) [4]int16 {
	out := Transform4(in)
	for i := range out {
		out[i] >>= 2
	}
	return out
}

func Transform(in []int16) {
	n := len(in)

	if n < 1 {
		return
	}
	if (n & (n - 1)) != 0 {
		return
	}

	fwht(in, n)
}

func Invert(in []int16) {
	n := len(in)

	fwht(in, n)
	for i, v := range in {
		in[i] = v / int16(n)
	}
}

func fwht(in []int16, n int) {
	if n < 2 {
		return
	}

	half := n / 2

	fwht(in[:half], half)
	fwht(in[half:], half)

	for i := 0; i < half; i += 1 {
		a := in[i]
		b := in[i+half]
		in[i] = a + b
		in[i+half] = a - b
	}
}
