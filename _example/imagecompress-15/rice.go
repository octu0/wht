package main

import (
	"io"
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
			return err
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
			return err
		}
	}
	return nil
}

func (w *BitWriter) Flush() error {
	if 0 < w.bits {
		if _, err := w.out.Write([]byte{w.cache}); err != nil {
			return err
		}
	}
	return nil
}

func NewBitWriter(out io.Writer) *BitWriter {
	return &BitWriter{out: out}
}

type RiceWriter[T Unsigned] struct {
	bw        *BitWriter
	maxVal    T
	zeroCount T
	lastK     uint8
}

func (w *RiceWriter[T]) writePrimitive(val T, k uint8) error {
	m := T(1) << k
	q := val / m
	r := val % m

	for i := T(0); i < q; i++ {
		if err := w.bw.WriteBit(1); err != nil {
			return err
		}
	}
	if err := w.bw.WriteBit(0); err != nil {
		return err
	}

	return w.bw.WriteBits(uint16(r), k)
}

func (w *RiceWriter[T]) Write(val T, k uint8) error {
	w.lastK = k

	if val == 0 {
		if w.zeroCount == w.maxVal {
			if err := w.flushZeros(k); err != nil {
				return err
			}
		}
		w.zeroCount += 1
		return nil
	}

	if 0 < w.zeroCount {
		if err := w.flushZeros(k); err != nil {
			return err
		}
	}

	return w.writePrimitive(val, k)
}

func (w *RiceWriter[T]) flushZeros(k uint8) error {
	if w.zeroCount == 0 {
		return nil
	}
	if err := w.writePrimitive(0, k); err != nil {
		return err
	}
	if err := w.writePrimitive(w.zeroCount, k); err != nil {
		return err
	}

	w.zeroCount = 0
	return nil
}

func (w *RiceWriter[T]) Flush() error {
	if 0 < w.zeroCount {
		if err := w.flushZeros(w.lastK); err != nil {
			return err
		}
	}
	return w.bw.Flush()
}

func NewRiceWriter[T Unsigned](bw *BitWriter) *RiceWriter[T] {
	return &RiceWriter[T]{bw: bw, maxVal: 64}
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
			return 0, err
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
			return 0, err
		}
		val = (val << 1) | uint16(bit)
	}
	return val, nil
}

func NewBitReader(r io.Reader) *BitReader {
	return &BitReader{r: r}
}

type RiceReader[T Unsigned] struct {
	br           *BitReader
	pendingZeros int
}

func (r *RiceReader[T]) readPrimitive(k uint8) (T, error) {
	q := T(0)
	for {
		bit, err := r.br.ReadBit()
		if err != nil {
			return 0, err
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
	val := (q << k) | T(rem64)
	return val, nil
}

func (r *RiceReader[T]) Read(k uint8) (T, error) {
	if 0 < r.pendingZeros {
		r.pendingZeros -= 1
		return 0, nil
	}

	val, err := r.readPrimitive(k)
	if err != nil {
		return 0, err
	}

	if val == 0 {
		count, err := r.readPrimitive(k)
		if err != nil {
			return 0, err
		}

		r.pendingZeros = int(count) - 1
		return 0, nil
	}

	return val, nil
}

func NewRiceReader[T Unsigned](br *BitReader) *RiceReader[T] {
	return &RiceReader[T]{br: br}
}
