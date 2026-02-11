package main

import (
	"bytes"
	"math/rand"
	"testing"
	"time"
)

func TestBitWriterReader(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	bw := NewBitWriter(buf)

	bitsToWrite := []uint8{1, 0, 1, 1, 0}
	for _, b := range bitsToWrite {
		if err := bw.WriteBit(b); err != nil {
			t.Fatalf("WriteBit failed: %v", err)
		}
	}

	val16 := uint16(0xAAAA)
	if err := bw.WriteBits(val16, 16); err != nil {
		t.Fatalf("WriteBits failed: %v", err)
	}

	if err := bw.Flush(); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	br := NewBitReader(bytes.NewReader(buf.Bytes()))
	for i, want := range bitsToWrite {
		got, err := br.ReadBit()
		if err != nil {
			t.Fatalf("ReadBit failed at index %d: %v", i, err)
		}
		if got != want {
			t.Errorf("Index %d: got %d, want %d", i, got, want)
		}
	}

	gotVal16, err := br.ReadBits(16)
	if err != nil {
		t.Fatalf("ReadBits failed: %v", err)
	}
	if gotVal16 != val16 {
		t.Errorf("ReadBits: got %x, want %x", gotVal16, val16)
	}
}

func TestRiceRoundTripMinMax(t *testing.T) {
	cases := []struct {
		val uint16
		k   uint8
	}{
		{0, 0},      // 最小値, Unaryのみ
		{0, 5},      // 最小値, k=5
		{1, 0},      // 小さい値
		{10, 0},     // Unaryが長くなるケース
		{255, 8},    // 8ビット境界
		{1024, 10},  // k=10
		{65535, 15}, // uint16最大値, k=15 (k=16は 1<<16 でオーバーフローするためRice符号では通常避ける)
	}

	buf := bytes.NewBuffer(nil)
	bw := NewBitWriter(buf)
	rw := NewRiceWriter[uint16](bw)

	for _, c := range cases {
		if err := rw.Write(c.val, c.k); err != nil {
			t.Fatalf("Write(%d, %d) failed: %v", c.val, c.k, err)
		}
	}
	if err := rw.Flush(); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	br := NewBitReader(bytes.NewReader(buf.Bytes()))
	rr := NewRiceReader[uint16](br)

	for i, c := range cases {
		got, err := rr.ReadRice(c.k)
		if err != nil {
			t.Fatalf("ReadRice (case %d: val=%d, k=%d) failed: %v", i, c.val, c.k, err)
		}
		if got != c.val {
			t.Errorf("Case %d: got %d, want %d (k=%d)", i, got, c.val, c.k)
		}
	}
}

func TestRiceRandom(t *testing.T) {
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))

	const numTests = 10000
	type input struct {
		val uint16
		k   uint8
	}
	inputs := make([]input, numTests)

	buf := bytes.NewBuffer(nil)
	bw := NewBitWriter(buf)
	rw := NewRiceWriter[uint16](bw)

	for i := 0; i < numTests; i += 1 {
		k := uint8(rnd.Intn(16))

		val := uint16(rnd.Intn(65536))
		inputs[i] = input{val, k}
		if err := rw.Write(val, k); err != nil {
			t.Fatalf("Random Write failed at %d: %v", i, err)
		}
	}
	rw.Flush()

	br := NewBitReader(bytes.NewReader(buf.Bytes()))
	rr := NewRiceReader[uint16](br)

	for i, inp := range inputs {
		got, err := rr.ReadRice(inp.k)
		if err != nil {
			t.Fatalf("Random Read failed at %d: %v", i, err)
		}
		if got != inp.val {
			t.Errorf("Random test %d: got %d, want %d (k=%d)", i, got, inp.val, inp.k)
		}
	}
}

func TestRiceK0(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	bw := NewBitWriter(buf)
	rw := NewRiceWriter[uint16](bw)

	// 3 (uint16) を k=0 で書く -> Unary(3) -> 1110 (4ビット)
	// 1 (uint16) を k=0 で書く -> Unary(1) -> 10 (2ビット)
	// 合計 111010xx (Flushで埋まる) -> 11101000 (0xE8)
	rw.Write(3, 0)
	rw.Write(1, 0)
	rw.Flush()

	b := buf.Bytes()
	if len(b) != 1 {
		t.Fatalf("Expected 1 byte, got %d", len(b))
	}
	expected := byte(0xE8) // 11101000
	if b[0] != expected {
		t.Errorf("Byte content mismatch: got %08b, want %08b", b[0], expected)
	}
}
