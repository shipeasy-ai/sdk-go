package shipeasy

const (
	c1 uint32 = 0xcc9e2d51
	c2 uint32 = 0x1b873593
)

func Murmur3(key string) uint32 {
	data := []byte(key)
	n := len(data)
	var h1 uint32 = 0
	nblocks := n / 4

	for i := 0; i < nblocks; i++ {
		off := i * 4
		k1 := uint32(data[off]) |
			uint32(data[off+1])<<8 |
			uint32(data[off+2])<<16 |
			uint32(data[off+3])<<24
		k1 *= c1
		k1 = (k1 << 15) | (k1 >> 17)
		k1 *= c2
		h1 ^= k1
		h1 = (h1 << 13) | (h1 >> 19)
		h1 = h1*5 + 0xe6546b64
	}

	tail := nblocks * 4
	var k1 uint32
	switch n & 3 {
	case 3:
		k1 ^= uint32(data[tail+2]) << 16
		fallthrough
	case 2:
		k1 ^= uint32(data[tail+1]) << 8
		fallthrough
	case 1:
		k1 ^= uint32(data[tail])
		k1 *= c1
		k1 = (k1 << 15) | (k1 >> 17)
		k1 *= c2
		h1 ^= k1
	}

	h1 ^= uint32(n)
	h1 ^= h1 >> 16
	h1 *= 0x85ebca6b
	h1 ^= h1 >> 13
	h1 *= 0xc2b2ae35
	h1 ^= h1 >> 16
	return h1
}
