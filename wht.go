package wht

func Transform4[T SignedInt](in [4]T) [4]T {
	return [4]T{
		(in[0] + in[1] + in[2] + in[3]),
		(in[0] + in[1] - in[2] - in[3]),
		(in[0] - in[1] - in[2] + in[3]),
		(in[0] - in[1] + in[2] - in[3]),
	}
}

func Invert4[T SignedInt](in [4]T) [4]T {
	out := Transform4(in)
	for i := range out {
		out[i] >>= 2
	}
	return out
}

func Transform[T Signed](in []T) {
	n := len(in)

	if n < 1 {
		return
	}
	if (n & (n - 1)) != 0 {
		return
	}

	fwht(in, n)
}

func Invert[T Signed](in []T) {
	n := len(in)

	fwht(in, n)
	for i, v := range in {
		in[i] = v / T(n)
	}
}

func fwht[T Signed](in []T, n int) {
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
