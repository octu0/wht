package main

import (
	"encoding/binary"
	"io"
	"unsafe"

	"github.com/octu0/wht"
	"github.com/pkg/errors"
)

func dcEncode(out io.Writer, dc []int16, size int) error {
	for i := 0; i < size; i += 1 {
		if err := binary.Write(out, binary.BigEndian, dc[i]); err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

func acEncode(out io.Writer, zigzag []int8) error {
	// int8 -> uint8
	data := unsafe.Slice((*uint8)(unsafe.Pointer(&zigzag[0])), len(zigzag))
	rw := NewRiceWriter[uint8](NewBitWriter(out))
	if err := rleEncode(rw, data); err != nil {
		return errors.WithStack(err)
	}
	if err := rw.Flush(); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func transform(out io.Writer, dcQT, acQT QuantizationTable, data [][]int16, size, scale int) error {
	if err := binary.Write(out, binary.BigEndian, uint8(scale)); err != nil {
		return errors.WithStack(err)
	}

	dc := make([]int16, size)
	for i := 0; i < size; i += 1 {
		wht.Transform(data[i])
		dc[i] = data[i][0]
	}
	wht.Transform(dc)
	dcQuantize(dc, dcQT)

	if err := dcEncode(out, dc, size); err != nil {
		return errors.WithStack(err)
	}

	qACList := make([][]int8, size)
	for i := 0; i < size; i += 1 {
		qACList[i] = acQuantize(data[i][1:], acQT, scale)
	}

	zigzag := Zigzag(qACList, size, size-1)
	if err := acEncode(out, zigzag); err != nil {
		return errors.WithStack(err)
	}
	return nil
}
