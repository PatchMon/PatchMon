package utils

import (
	"bytes"
	"sync"
)

// BufferPool provides a pool of reusable byte buffers to reduce allocations
var BufferPool = sync.Pool{
	New: func() interface{} {
		// Create a buffer with 4KB initial capacity
		return bytes.NewBuffer(make([]byte, 0, 4096))
	},
}

// GetBuffer retrieves a buffer from the pool
func GetBuffer() *bytes.Buffer {
	return BufferPool.Get().(*bytes.Buffer)
}

// PutBuffer returns a buffer to the pool after resetting it
func PutBuffer(buf *bytes.Buffer) {
	if buf != nil {
		buf.Reset()
		BufferPool.Put(buf)
	}
}

// ByteSlicePool provides a pool of reusable byte slices
var ByteSlicePool = sync.Pool{
	New: func() interface{} {
		b := make([]byte, 4096)
		return &b
	},
}

// GetByteSlice retrieves a byte slice from the pool
func GetByteSlice() *[]byte {
	return ByteSlicePool.Get().(*[]byte)
}

// PutByteSlice returns a byte slice to the pool
func PutByteSlice(b *[]byte) {
	if b != nil {
		ByteSlicePool.Put(b)
	}
}
