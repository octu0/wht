package main

import (
	"io"

	"github.com/pkg/errors"
)

const (
	keyK   = 7
	valueK = 5
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
