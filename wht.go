package wht

func Transform4[T SignedInt](in [4]T) [4]T {
	return [4]T{
		(in[0] + in[1] + in[2] + in[3]),
		(in[0] + in[1] - in[2] - in[3]),
		(in[0] - in[1] - in[2] + in[3]),
		(in[0] - in[1] + in[2] - in[3]),
	}
}

func Transform8[T SignedInt](in [8]T) [8]T {
	a0 := in[0] + in[1]
	a1 := in[0] - in[1]
	a2 := in[2] + in[3]
	a3 := in[2] - in[3]
	a4 := in[4] + in[5]
	a5 := in[4] - in[5]
	a6 := in[6] + in[7]
	a7 := in[6] - in[7]

	b0 := a0 + a2
	b1 := a1 + a3
	b2 := a0 - a2
	b3 := a1 - a3
	b4 := a4 + a6
	b5 := a5 + a7
	b6 := a4 - a6
	b7 := a5 - a7
	return [8]T{
		b0 + b4,
		b1 + b5,
		b2 + b6,
		b3 + b7,
		b0 - b4,
		b1 - b5,
		b2 - b6,
		b3 - b7,
	}
}

func Transform16[T SignedInt](in [16]T) [16]T {
	a0 := in[0] + in[1]
	a1 := in[0] - in[1]
	a2 := in[2] + in[3]
	a3 := in[2] - in[3]
	a4 := in[4] + in[5]
	a5 := in[4] - in[5]
	a6 := in[6] + in[7]
	a7 := in[6] - in[7]
	a8 := in[8] + in[9]
	a9 := in[8] - in[9]
	a10 := in[10] + in[11]
	a11 := in[10] - in[11]
	a12 := in[12] + in[13]
	a13 := in[12] - in[13]
	a14 := in[14] + in[15]
	a15 := in[14] - in[15]

	b0 := a0 + a2
	b1 := a1 + a3
	b2 := a0 - a2
	b3 := a1 - a3
	b4 := a4 + a6
	b5 := a5 + a7
	b6 := a4 - a6
	b7 := a5 - a7
	b8 := a8 + a10
	b9 := a9 + a11
	b10 := a8 - a10
	b11 := a9 - a11
	b12 := a12 + a14
	b13 := a13 + a15
	b14 := a12 - a14
	b15 := a13 - a15

	c0 := b0 + b4
	c1 := b1 + b5
	c2 := b2 + b6
	c3 := b3 + b7
	c4 := b0 - b4
	c5 := b1 - b5
	c6 := b2 - b6
	c7 := b3 - b7
	c8 := b8 + b12
	c9 := b9 + b13
	c10 := b10 + b14
	c11 := b11 + b15
	c12 := b8 - b12
	c13 := b9 - b13
	c14 := b10 - b14
	c15 := b11 - b15

	return [16]T{
		c0 + c8,
		c1 + c9,
		c2 + c10,
		c3 + c11,
		c4 + c12,
		c5 + c13,
		c6 + c14,
		c7 + c15,
		c0 - c8,
		c1 - c9,
		c2 - c10,
		c3 - c11,
		c4 - c12,
		c5 - c13,
		c6 - c14,
		c7 - c15,
	}
}

func Invert4[T SignedInt](in [4]T) [4]T {
	out := Transform4(in)
	for i := range out {
		out[i] >>= 2
	}
	return out
}

func Invert8[T SignedInt](in [8]T) [8]T {
	out := Transform8(in)
	for i := range out {
		out[i] >>= 3
	}
	return out
}

func Invert16[T SignedInt](in [16]T) [16]T {
	out := Transform16(in)
	for i := range out {
		out[i] >>= 4
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
