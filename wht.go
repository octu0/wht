package wht

import (
	"math/bits"
)

func Transform4[T SignedInt](in [4]T) [4]T {
	return [4]T{
		(in[0] + in[1] + in[2] + in[3]), // Seq 0 (Nat 0)
		(in[0] + in[1] - in[2] - in[3]), // Seq 1 (Nat 2)
		(in[0] - in[1] - in[2] + in[3]), // Seq 2 (Nat 3)
		(in[0] - in[1] + in[2] - in[3]), // Seq 3 (Nat 1)
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
		b0 + b4, // Seq 0 (Nat 0)
		b0 - b4, // Seq 1 (Nat 4)
		b2 - b6, // Seq 2 (Nat 6)
		b2 + b6, // Seq 3 (Nat 2)
		b3 + b7, // Seq 4 (Nat 3)
		b3 - b7, // Seq 5 (Nat 7)
		b1 - b5, // Seq 6 (Nat 5)
		b1 + b5, // Seq 7 (Nat 1)
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
		c0 + c8,  // Seq 0  (Nat 0)
		c0 - c8,  // Seq 1  (Nat 8)
		c4 - c12, // Seq 2  (Nat 12)
		c4 + c12, // Seq 3  (Nat 4)
		c6 + c14, // Seq 4  (Nat 6)
		c6 - c14, // Seq 5  (Nat 14)
		c2 - c10, // Seq 6  (Nat 10)
		c2 + c10, // Seq 7  (Nat 2)
		c3 + c11, // Seq 8  (Nat 3)
		c3 - c11, // Seq 9  (Nat 11)
		c7 - c15, // Seq 10 (Nat 15)
		c7 + c15, // Seq 11 (Nat 7)
		c5 + c13, // Seq 12 (Nat 5)
		c5 - c13, // Seq 13 (Nat 13)
		c1 - c9,  // Seq 14 (Nat 9)
		c1 + c9,  // Seq 15 (Nat 1)
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
	// Remap Sequency Order input to Natural Order for butterfly
	a0 := in[0] + in[7] // in[0]=Nat0, in[7]=Nat1
	a1 := in[0] - in[7]
	a2 := in[3] + in[4] // in[3]=Nat2, in[4]=Nat3
	a3 := in[3] - in[4]
	a4 := in[1] + in[6] // in[1]=Nat4, in[6]=Nat5
	a5 := in[1] - in[6]
	a6 := in[2] + in[5] // in[2]=Nat6, in[5]=Nat7
	a7 := in[2] - in[5]

	b0 := a0 + a2
	b1 := a1 + a3
	b2 := a0 - a2
	b3 := a1 - a3
	b4 := a4 + a6
	b5 := a5 + a7
	b6 := a4 - a6
	b7 := a5 - a7

	return [8]T{
		(b0 + b4) >> 3,
		(b1 + b5) >> 3,
		(b2 + b6) >> 3,
		(b3 + b7) >> 3,
		(b0 - b4) >> 3,
		(b1 - b5) >> 3,
		(b2 - b6) >> 3,
		(b3 - b7) >> 3,
	}
}

func Invert16[T SignedInt](in [16]T) [16]T {
	// Remap Sequency Order input to Natural Order for butterfly
	a0 := in[0] + in[15] // in[0]=Nat0, in[15]=Nat1
	a1 := in[0] - in[15]
	a2 := in[7] + in[8] // in[7]=Nat2, in[8]=Nat3
	a3 := in[7] - in[8]
	a4 := in[3] + in[12] // in[3]=Nat4, in[12]=Nat5
	a5 := in[3] - in[12]
	a6 := in[4] + in[11] // in[4]=Nat6, in[11]=Nat7
	a7 := in[4] - in[11]
	a8 := in[1] + in[14] // in[1]=Nat8, in[14]=Nat9
	a9 := in[1] - in[14]
	a10 := in[6] + in[9] // in[6]=Nat10, in[9]=Nat11
	a11 := in[6] - in[9]
	a12 := in[2] + in[13] // in[2]=Nat12, in[13]=Nat13
	a13 := in[2] - in[13]
	a14 := in[5] + in[10] // in[5]=Nat14, in[10]=Nat15
	a15 := in[5] - in[10]

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
		(c0 + c8) >> 4,
		(c1 + c9) >> 4,
		(c2 + c10) >> 4,
		(c3 + c11) >> 4,
		(c4 + c12) >> 4,
		(c5 + c13) >> 4,
		(c6 + c14) >> 4,
		(c7 + c15) >> 4,
		(c0 - c8) >> 4,
		(c1 - c9) >> 4,
		(c2 - c10) >> 4,
		(c3 - c11) >> 4,
		(c4 - c12) >> 4,
		(c5 - c13) >> 4,
		(c6 - c14) >> 4,
		(c7 - c15) >> 4,
	}
}

// Transform applies Walsh-Hadamard Transform to a slice of any size 2^n.
// The output is reordered to Sequency Order.
func Transform[T Signed](in []T) {
	n := len(in)
	if n < 1 {
		return
	}
	if (n & (n - 1)) != 0 {
		return
	}

	// 1. In-place Fast Walsh-Hadamard Transform (Natural Order)
	fwht(in, n)

	// 2. Permutation to Sequency Order
	// The mapping from Sequency Order index 'k' to Natural Order index 'nat'
	// is given by BitReverse(GrayCode(k)).
	temp := make([]T, n)
	bitsLen := bits.Len(uint(n)) - 1
	for k := 0; k < n; k += 1 {
		g := k ^ (k >> 1)
		nat := bits.Reverse(uint(g)) >> (64 - bitsLen)
		temp[k] = in[nat]
	}
	copy(in, temp)
}

// Invert applies Inverse Walsh-Hadamard Transform to a slice of any size 2^n.
// Assumes the input is in Sequency Order.
func Invert[T Signed](in []T) {
	n := len(in)
	if n < 1 {
		return
	}

	// 1. Permutation from Sequency Order back to Natural Order
	temp := make([]T, n)
	bitsLen := bits.Len(uint(n)) - 1
	for k := 0; k < n; k += 1 {
		g := k ^ (k >> 1)
		nat := bits.Reverse(uint(g)) >> (64 - bitsLen)
		temp[nat] = in[k]
	}
	copy(in, temp)

	// 2. Inverse FWHT (Natural Order)
	fwht(in, n)

	// 3. Normalization
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
