package embedding

import (
	"context"
	"encoding/binary"
	"errors"
	"hash/fnv"
	"math"
	"strings"
	"unicode"
)

type LocalHash struct {
	dimension int
}

func NewLocalHash(dimension int) *LocalHash {
	return &LocalHash{dimension: dimension}
}

func (e *LocalHash) Name() string {
	return "local-hash-v1"
}

func (e *LocalHash) Dimension() int {
	return e.dimension
}

func (e *LocalHash) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if e.dimension < 1 {
		return nil, errors.New("embedding dimension must be positive")
	}
	vectors := make([][]float32, len(texts))
	for index, text := range texts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		vectors[index] = e.embedOne(text)
	}
	return vectors, nil
}

func (e *LocalHash) embedOne(text string) []float32 {
	vector := make([]float32, e.dimension)
	tokens := features(strings.ToLower(strings.TrimSpace(text)))
	for _, token := range tokens {
		hasher := fnv.New64a()
		_, _ = hasher.Write([]byte(token))
		sum := hasher.Sum64()
		position := int(sum % uint64(e.dimension))
		sign := float32(1)
		var raw [8]byte
		binary.LittleEndian.PutUint64(raw[:], sum)
		if raw[7]&1 == 1 {
			sign = -1
		}
		vector[position] += sign
	}

	var norm float64
	for _, value := range vector {
		norm += float64(value * value)
	}
	if norm == 0 {
		return vector
	}
	scale := float32(1 / math.Sqrt(norm))
	for index := range vector {
		vector[index] *= scale
	}
	return vector
}

func features(text string) []string {
	runes := []rune(text)
	result := make([]string, 0, len(runes)*2)
	var word []rune
	flushWord := func() {
		if len(word) > 0 {
			result = append(result, string(word))
			word = word[:0]
		}
	}

	var compact []rune
	for _, current := range runes {
		if unicode.IsLetter(current) || unicode.IsNumber(current) {
			word = append(word, current)
			compact = append(compact, current)
			continue
		}
		flushWord()
	}
	flushWord()

	for size := 2; size <= 3; size++ {
		for start := 0; start+size <= len(compact); start++ {
			result = append(result, string(compact[start:start+size]))
		}
	}
	return result
}
