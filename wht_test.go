package wht

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestTransformInline(t *testing.T) {
	t.Run("4", func(tt *testing.T) {
		x := [4]int16{138, 144, 149, 153}
		r := Transform4(x)
		y := Invert4(r)
		if cmp.Equal(x, y) != true {
			tt.Errorf("%v != %v", x, y)
		}
	})
	t.Run("8", func(tt *testing.T) {
		x := [8]int16{20, 35, 41, 58, 66, 79, 81, 93}
		r := Transform8(x)
		y := Invert8(r)
		if cmp.Equal(x, y) != true {
			tt.Errorf("%v != %v", x, y)
		}
	})
	t.Run("16", func(tt *testing.T) {
		x := [16]int16{
			10, 20, 30, 40, 50, 60, 70, 80,
			90, 85, 75, 65, 55, 45, 35, 25,
		}
		r := Transform16(x)
		y := Invert16(r)
		if cmp.Equal(x, y) != true {
			tt.Errorf("%v != %v", x, y)
		}
	})
}

func TestTransform(t *testing.T) {
	t.Run("simple", func(tt *testing.T) {
		x := []int16{1, 0, 1, 0, 0, 1, 1, 0}
		Transform(x)
		expect1 := []int16{4, 2, 0, -2, 0, 2, 0, 2}
		if cmp.Equal(x, expect1) != true {
			tt.Errorf("%v != %v", x, expect1)
		}
		Invert(x)
		expect2 := []int16{1, 0, 1, 0, 0, 1, 1, 0}
		if cmp.Equal(x, expect2) != true {
			tt.Errorf("%v != %v", x, expect2)
		}
	})
	t.Run("impulse", func(tt *testing.T) {
		x := []int16{8, 0, 0, 0, 0, 0, 0, 0}
		Transform(x)
		expect1 := []int16{8, 8, 8, 8, 8, 8, 8, 8}
		if cmp.Equal(x, expect1) != true {
			tt.Errorf("%v != %v", x, expect1)
		}
		Invert(x)
		expect2 := []int16{8, 0, 0, 0, 0, 0, 0, 0}
		if cmp.Equal(x, expect2) != true {
			tt.Errorf("%v != %v", x, expect2)
		}
	})
	t.Run("dc", func(tt *testing.T) {
		x := []int16{5, 5, 5, 5, 5, 5, 5, 5}
		Transform(x)
		expect1 := []int16{40, 0, 0, 0, 0, 0, 0, 0}
		if cmp.Equal(x, expect1) != true {
			tt.Errorf("%v != %v", x, expect1)
		}
		Invert(x)
		expect2 := []int16{5, 5, 5, 5, 5, 5, 5, 5}
		if cmp.Equal(x, expect2) != true {
			tt.Errorf("%v != %v", x, expect2)
		}
	})
	t.Run("highseq", func(tt *testing.T) {
		x := []int16{1, -1, 1, -1, 1, -1, 1, -1}
		Transform(x)
		expect1 := []int16{0, 8, 0, 0, 0, 0, 0, 0}
		if cmp.Equal(x, expect1) != true {
			tt.Errorf("%v != %v", x, expect1)
		}
		Invert(x)
		expect2 := []int16{1, -1, 1, -1, 1, -1, 1, -1}
		if cmp.Equal(x, expect2) != true {
			tt.Errorf("%v != %v", x, expect2)
		}
	})
	t.Run("sin", func(tt *testing.T) {
		x := []int16{0, 7, 10, 7, 0, -7, -10, -7}
		Transform(x)
		expect1 := []int16{0, 0, 0, 0, 48, -8, -20, -20}
		if cmp.Equal(x, expect1) != true {
			tt.Errorf("%v != %v", x, expect1)
		}
		Invert(x)
		expect2 := []int16{0, 7, 10, 7, 0, -7, -10, -7}
		if cmp.Equal(x, expect2) != true {
			tt.Errorf("%v != %v", x, expect2)
		}
	})
}
