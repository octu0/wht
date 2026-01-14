package main

import (
	"io"

	"github.com/pkg/errors"
)

const (
	keyK   = 7
	valueK = 5

	dcKeyK   = 1
	dcValueK = 5
)

func rleEncode(rw *RiceWriter[uint8], data []uint8) error {
	currentValue := data[0]
	currentLength := byte(1)

	for i := 1; i < len(data); i += 1 {
		if data[i] != currentValue || currentLength == 255 {
			if err := rw.Write(currentLength, keyK); err != nil {
				return errors.Wrapf(err, "failed to write data: len=%d, val=%v", currentLength, currentValue)
			}
			if err := rw.Write(currentValue, valueK); err != nil {
				return errors.Wrapf(err, "failed to write data: len=%d, val=%v", currentLength, currentValue)
			}
			currentValue = data[i]
			currentLength = 1
		} else {
			currentLength += 1
		}
	}
	if err := rw.Write(currentLength, keyK); err != nil {
		return errors.Wrapf(err, "failed to write data:%d %v", currentLength, currentValue)
	}
	if err := rw.Write(currentValue, valueK); err != nil {
		return errors.Wrapf(err, "failed to write data:%d %v", currentLength, currentValue)
	}
	return nil
}

func rleDecode(rw *RiceReader[uint8], rows, cols int) ([]uint8, error) {
	out := make([]uint8, 0, rows*cols)
	for {
		lenVal, err := rw.ReadRice(keyK)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, errors.WithStack(err)
		}
		valVal, err := rw.ReadRice(valueK)
		if err != nil {
			return nil, errors.WithStack(err)
		}

		for i := byte(0); i < lenVal; i += 1 {
			out = append(out, valVal)
		}
	}
	return out, nil
}

func zigzagEncode(n int16) uint16 {
	return uint16((n << 1) ^ (n >> 15))
}

func zigzagDecode(n uint16) int16 {
	return int16((n >> 1) ^ -(n & 1))
}

func dcRleEncode(rw *RiceWriter[uint16], data []int16) error {
	if len(data) == 0 {
		return nil
	}
	currentVal := zigzagEncode(data[0])
	currentLen := uint16(1)

	for i := 1; i < len(data); i += 1 {
		v := zigzagEncode(data[i])
		if v != currentVal {
			if err := rw.Write(currentLen, dcKeyK); err != nil {
				return errors.Wrapf(err, "failed to write dc data: len=%d, val=%v", currentLen, currentVal)
			}
			if err := rw.Write(currentVal, dcValueK); err != nil {
				return errors.Wrapf(err, "failed to write dc data: len=%d, val=%v", currentLen, currentVal)
			}
			currentVal = v
			currentLen = 1
		} else {
			currentLen += 1
		}
	}
	if err := rw.Write(currentLen, dcKeyK); err != nil {
		return errors.Wrapf(err, "failed to write dc data: len=%d, val=%v", currentLen, currentVal)
	}
	if err := rw.Write(currentVal, dcValueK); err != nil {
		return errors.Wrapf(err, "failed to write dc data: len=%d, val=%v", currentLen, currentVal)
	}
	return nil
}

func dcRleDecode(rw *RiceReader[uint16], size int) ([]int16, error) {
	out := make([]int16, 0, size)
	for len(out) < size {
		lenVal, err := rw.ReadRice(dcKeyK)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		valVal, err := rw.ReadRice(dcValueK)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		decodedVal := zigzagDecode(valVal)
		for i := uint16(0); i < lenVal; i += 1 {
			out = append(out, decodedVal)
		}
	}
	return out, nil
}
