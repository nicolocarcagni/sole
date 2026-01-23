package main

import (
	"crypto/sha256"
	"errors"
	"math/big"
)

const b58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

// Base58Encode encodes a byte array to Base58
func Base58Encode(input []byte) []byte {
	var result []byte

	x := new(big.Int).SetBytes(input)

	base := big.NewInt(int64(len(b58Alphabet)))
	zero := big.NewInt(0)
	mod := &big.Int{}

	for x.Cmp(zero) != 0 {
		x.DivMod(x, base, mod)
		result = append(result, b58Alphabet[mod.Int64()])
	}

	ReverseBytes(result)

	for _, b := range input {
		if b == 0x00 {
			result = append([]byte{b58Alphabet[0]}, result...)
		} else {
			break
		}
	}

	return result
}

// Base58Decode decodes a Base58 encoded byte array
func Base58Decode(input []byte) ([]byte, error) {
	result := big.NewInt(0)
	zeroBytes := 0

	for _, b := range input {
		if b == b58Alphabet[0] {
			zeroBytes++
		} else {
			break
		}
	}

	payload := input[zeroBytes:]
	for _, b := range payload {
		charIndex := -1
		for i := 0; i < len(b58Alphabet); i++ {
			if b == b58Alphabet[i] {
				charIndex = i
				break
			}
		}

		if charIndex < 0 {
			return []byte{}, errors.New("invalid base58 character")
		}

		result.Mul(result, big.NewInt(58))
		result.Add(result, big.NewInt(int64(charIndex)))
	}

	decoded := result.Bytes()
	resultBytes := append(make([]byte, zeroBytes), decoded...)

	return resultBytes, nil
}

// ReverseBytes reverses a byte array
func ReverseBytes(data []byte) {
	for i, j := 0, len(data)-1; i < j; i, j = i+1, j-1 {
		data[i], data[j] = data[j], data[i]
	}
}

// checksum calculates the checksum of the input
func checksum(payload []byte) []byte {
	firstSHA := sha256.Sum256(payload)
	secondSHA := sha256.Sum256(firstSHA[:])
	return secondSHA[:4]
}
