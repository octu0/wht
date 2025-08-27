package wht

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestZigzag(t *testing.T) {
	t.Run("0..15", func(tt *testing.T) {
		stride := 4
		// 4x4
		data := []int16{
			0, 1, 5, 6,
			2, 4, 7, 12,
			3, 8, 11, 13,
			9, 10, 14, 15,
		}

		sorted := Zigzag(data, stride)
		expect1 := []int16{
			0, 1, 2, 3,
			4, 5, 6, 7,
			8, 9, 10, 11,
			12, 13, 14, 15,
		}
		if cmp.Equal(sorted, expect1) != true {
			tt.Errorf("%v != %v", sorted, expect1)
		}

		restored := Unzigzag(sorted, stride)
		expect2 := []int16{
			0, 1, 5, 6,
			2, 4, 7, 12,
			3, 8, 11, 13,
			9, 10, 14, 15,
		}
		if cmp.Equal(restored, expect2) != true {
			tt.Errorf("%v != %v", restored, expect2)
		}
	})
}
