package main

import (
	"io"

	"github.com/pkg/errors"
)

type Unsigned interface {
	uint8 | uint16
}

type BitWriter struct {
	out   io.Writer
	cache byte
	bits  uint8
}

func (w *BitWriter) WriteBit(bit uint8) error {
	if 0 < bit {
		w.cache |= (1 << (7 - w.bits))
	}
	w.bits += 1
	if w.bits == 8 {
		if _, err := w.out.Write([]byte{w.cache}); err != nil {
			return errors.WithStack(err)
		}
		w.bits = 0
		w.cache = 0
	}
	return nil
}

func (w *BitWriter) WriteBits(val uint16, n uint8) error {
	for i := uint8(0); i < n; i += 1 {
		bit := (val >> (n - 1 - i)) & 1
		if err := w.WriteBit(uint8(bit)); err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

func (w *BitWriter) Flush() error {
	if 0 < w.bits {
		if _, err := w.out.Write([]byte{w.cache}); err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

func NewBitWriter(out io.Writer) *BitWriter {
	return &BitWriter{out: out}
}

type BitReader struct {
	r     io.Reader
	cache byte
	bits  uint8
}

func (r *BitReader) ReadBit() (uint8, error) {
	if r.bits == 0 {
		buf := make([]byte, 1)
		if _, err := io.ReadFull(r.r, buf); err != nil {
			return 0, errors.WithStack(err)
		}
		r.cache = buf[0]
		r.bits = 8
	}
	r.bits -= 1
	bit := (r.cache >> r.bits) & 1
	return bit, nil
}

func (r *BitReader) ReadBits(n uint8) (uint16, error) {
	val := uint16(0)
	for i := uint8(0); i < n; i += 1 {
		bit, err := r.ReadBit()
		if err != nil {
			return 0, errors.WithStack(err)
		}
		val = (val << 1) | uint16(bit)
	}
	return val, nil
}

func NewBitReader(r io.Reader) *BitReader {
	return &BitReader{r: r}
}

type RiceWriter[T Unsigned] struct {
	bw *BitWriter
}

func (w *RiceWriter[T]) Write(val T, k uint8) error {
	m := T(1) << k
	q := val / m
	r := val % m

	for i := T(0); i < q; i += 1 {
		if err := w.bw.WriteBit(1); err != nil {
			return errors.WithStack(err)
		}
	}
	if err := w.bw.WriteBit(0); err != nil {
		return err
	}

	return w.bw.WriteBits(uint16(r), k)
}

func (w *RiceWriter[T]) Flush() error {
	return w.bw.Flush()
}

func NewRiceWriter[T Unsigned](bw *BitWriter) *RiceWriter[T] {
	return &RiceWriter[T]{bw: bw}
}

type RiceReader[T Unsigned] struct {
	br *BitReader
}

func (r *RiceReader[T]) ReadRice(k uint8) (T, error) {
	q := T(0)
	for {
		bit, err := r.br.ReadBit()
		if err != nil {
			return 0, errors.WithStack(err)
		}
		if bit == 0 {
			break
		}
		q += 1
	}

	rem64, err := r.br.ReadBits(k)
	if err != nil {
		return 0, err
	}
	rem := T(rem64)

	val := (q << k) | rem
	return val, nil
}

func NewRiceReader[T Unsigned](br *BitReader) *RiceReader[T] {
	return &RiceReader[T]{br: br}
}
