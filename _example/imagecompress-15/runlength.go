package main

import (
	"github.com/pkg/errors"
)

const (
	blockKeyK   = 1
	blockValueK = 15
)

func blockRLEEncode(rw *RiceWriter[uint16], data []uint16) error {
	if len(data) == 0 {
		return nil
	}
	currentVal := data[0]
	currentLen := uint16(1)

	for i := 1; i < len(data); i += 1 {
		v := data[i]
		if v != currentVal || currentLen == 65535 {
			if err := rw.Write(currentLen, blockKeyK); err != nil {
				return errors.WithStack(err)
			}
			if err := rw.Write(currentVal, blockValueK); err != nil {
				return errors.WithStack(err)
			}
			currentVal = v
			currentLen = 1
		} else {
			currentLen += 1
		}
	}
	if err := rw.Write(currentLen, blockKeyK); err != nil {
		return errors.WithStack(err)
	}
	if err := rw.Write(currentVal, blockValueK); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func blockRLEDecode(rw *RiceReader[uint16], size uint16) ([]uint16, error) {
	out := make([]uint16, 0, size)
	for len(out) < int(size) {
		lenVal, err := rw.ReadRice(blockKeyK)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		valVal, err := rw.ReadRice(blockValueK)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		for i := uint16(0); i < lenVal; i += 1 {
			out = append(out, valVal)
		}
	}
	return out, nil
}
