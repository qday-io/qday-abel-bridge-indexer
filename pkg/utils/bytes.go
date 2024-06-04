package utils

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"
)

// Define data types.
type Bytes []byte

// Define methods for Bytes.
func AsBytes(data []byte) Bytes {
	return Bytes(data)
}

func MakeBytes(length int, capacity ...int) Bytes {
	if len(capacity) == 0 {
		capacity = []int{length}
	}
	return AsBytes(make([]byte, length, capacity[0]))
}

func MakeBytesFromHexString(hexString string) Bytes {
	b, err := hex.DecodeString(hexString)
	if err != nil {
		panic(err)
	}
	return AsBytes(b)
}

func MakeRandomBytes(length int, seed ...int64) Bytes {
	if len(seed) == 0 {
		seed = []int64{-1}
	}
	b := MakeBytes(length)
	b.FillRandomData(seed[0], "")
	return b
}

func (b Bytes) Slice() []byte {
	return ([]byte)(b)
}

func (b Bytes) Len() int {
	return len(b.Slice())
}

func (b Bytes) HexString() string {
	return hex.EncodeToString(b.Slice())
}

func (b Bytes) Base64String() string {
	return base64.StdEncoding.EncodeToString(b.Slice())
}

func (b Bytes) Md5() Bytes {
	hash := md5.Sum(b.Slice())
	return AsBytes(hash[:])
}

func (b Bytes) Sha256() Bytes {
	hash := sha256.Sum256(b.Slice())
	return AsBytes(hash[:])
}

func (b Bytes) String() string {
	return b.Summary(2, 8)
}

func (b Bytes) Summary(verbosity int, affixLen ...int) string {
	if len(affixLen) == 0 {
		affixLen = []int{8}
	}

	var body string
	if b.Len() == 0 {
		body = "∅"
	} else if b.Len() <= 2*affixLen[0] {
		body = b.HexString()
	} else {
		prefix := AsBytes(b.Slice()[0:affixLen[0]])
		suffix := AsBytes(b.Slice()[b.Len()-affixLen[0] : b.Len()])
		body = fmt.Sprintf("%s..%s", prefix.HexString(), suffix.HexString())
	}

	if verbosity == 0 {
		return body
	}

	body = fmt.Sprintf("len:%d|data:%s|md5:%s", b.Len(), body, b.Md5().Summary(0, 2))
	if verbosity == 1 {
		return body
	} else {
		return fmt.Sprintf("Bytes{%s}", body)
	}
}

func (b Bytes) FillRandomData(seed int64, alphabet string) {
	// Panic if not all bytes are zero.
	for _, v := range b.Slice() {
		if v != 0 {
			panic("only all-zero Bytes can be filled with random data")
		}
	}

	// Set random seed.
	if seed < 0 {
		seed = time.Now().UTC().UnixNano()
	}
	rand.Seed(seed)

	// Fill each byte with random data.
	data := b.Slice()
	if alphabet == "" {
		// If alphabet is empty, use all available values.
		for i := range data {
			data[i] = byte(rand.Intn(256))
		}
	} else {
		// Otherwise, use the given alphabet.
		for i := range data {
			data[i] = alphabet[rand.Intn(len(alphabet))]
		}
	}
}

func (b Bytes) JSONUnmarshal(v any) error {
	return json.Unmarshal(b.Slice(), v)
}
