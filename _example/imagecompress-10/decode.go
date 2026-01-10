package main

import (
	"encoding/binary"
	"io"
	"unsafe"

	"github.com/octu0/wht"
	"github.com/pkg/errors"
)

func dcDecode(in io.Reader, size int) ([]int16, error) {
	dc := make([]int16, size)
	for i := 0; i < size; i += 1 {
		if err := binary.Read(in, binary.BigEndian, &dc[i]); err != nil {
			return nil, errors.WithStack(err)
		}
	}
	return dc, nil
}

func acDecode(in io.Reader, rows, cols int) ([]int8, error) {
	rw := NewRiceReader[uint8](NewBitReader(in))
	data, err := rleDecode(rw, rows, cols)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	// uint8 -> int8
	ac := unsafe.Slice((*int8)(unsafe.Pointer(&data[0])), len(data))
	return ac, nil
}

func invert(in io.Reader, dcQT, acQT QuantizationTable, size int) ([][]int16, error) {
	scale := uint8(0)
	if err := binary.Read(in, binary.BigEndian, &scale); err != nil {
		return nil, errors.WithStack(err)
	}

	dc, err := dcDecode(in, size)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	dcDequantize(dc, dcQT)
	wht.Invert(dc)

	zigzag, err := acDecode(in, size, size-1)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	acList := make([][]int16, size)
	acPlanes := Unzigzag(zigzag, size, size-1)
	for i := 0; i < size; i += 1 {
		ac := acDequantize(acPlanes[i], acQT, int(scale))
		acList[i] = make([]int16, size)
		acList[i][0] = dc[i]
		copy(acList[i][1:size], ac)
		wht.Invert(acList[i])
	}
	return acList, nil
}
