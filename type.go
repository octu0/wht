package wht

type SignedInt interface {
	~int8 | ~int16 | ~int32 | ~int64
}

type Signed interface {
	~int8 | ~int16 | ~int32 | ~int64 | ~float32 | ~float64
}
