package main

import (
	"io"

	"github.com/pkg/errors"
)

const (
	acKeyK   = 7
	acValueK = 5

	dcKeyK   = 1
	dcValueK = 15
)

func acRLEEncode(rw *RiceWriter[uint8], data []uint8) error {
	currentValue := data[0]
	currentLength := byte(1)

	for i := 1; i < len(data); i += 1 {
		if data[i] != currentValue || currentLength == 255 {
			if err := rw.Write(currentLength, acKeyK); err != nil {
				return errors.Wrapf(err, "failed to write data: len=%d, val=%v", currentLength, currentValue)
			}
			if err := rw.Write(currentValue, acValueK); err != nil {
				return errors.Wrapf(err, "failed to write data: len=%d, val=%v", currentLength, currentValue)
			}
			currentValue = data[i]
			currentLength = 1
		} else {
			currentLength += 1
		}
	}
	if err := rw.Write(currentLength, acKeyK); err != nil {
		return errors.Wrapf(err, "failed to write data:%d %v", currentLength, currentValue)
	}
	if err := rw.Write(currentValue, acValueK); err != nil {
		return errors.Wrapf(err, "failed to write data:%d %v", currentLength, currentValue)
	}
	return nil
}

func acRLEDecode(rw *RiceReader[uint8], rows, cols int) ([]uint8, error) {
	out := make([]uint8, 0, rows*cols)
	for {
		lenVal, err := rw.ReadRice(acKeyK)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, errors.WithStack(err)
		}
		valVal, err := rw.ReadRice(acValueK)
		if err != nil {
			return nil, errors.WithStack(err)
		}

		for i := byte(0); i < lenVal; i += 1 {
			out = append(out, valVal)
		}
	}
	return out, nil
}

func dcRLEEncode(rw *RiceWriter[uint16], data []uint16) error {
	if len(data) == 0 {
		return nil
	}
	currentVal := data[0]
	currentLen := uint16(1)

	for i := 1; i < len(data); i += 1 {
		v := data[i]
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

func dcRLEDecode(rw *RiceReader[uint16], size int) ([]uint16, error) {
	out := make([]uint16, 0, size)
	for len(out) < size {
		lenVal, err := rw.ReadRice(dcKeyK)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		valVal, err := rw.ReadRice(dcValueK)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		decodedVal := valVal
		for i := uint16(0); i < lenVal; i += 1 {
			out = append(out, decodedVal)
		}
	}
	return out, nil
}
