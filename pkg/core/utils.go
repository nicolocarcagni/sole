package core

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"golang.org/x/crypto/ripemd160"
)

func ExtractPubKeyHash(address string) ([]byte, error) {
	pubKeyHash, err := Base58Decode([]byte(address))
	if err != nil {
		return nil, err
	}
	if len(pubKeyHash) < 5 {
		return nil, fmt.Errorf("invalid address length")
	}
	return pubKeyHash[1 : len(pubKeyHash)-4], nil
}

func AddressFromPubKeyHash(pubKeyHash []byte) string {
	versionedPayload := append([]byte{version}, pubKeyHash...)
	chksum := Checksum(versionedPayload)
	fullPayload := append(versionedPayload, chksum...)
	return string(Base58Encode(fullPayload))
}

func IntToHex(num int64) []byte {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], uint64(num))
	return b[:]
}

func HashPubKey(pubKey []byte) []byte {
	publicSHA256 := sha256.Sum256(pubKey)

	RIPEMD160Hasher := ripemd160.New()
	_, _ = RIPEMD160Hasher.Write(publicSHA256[:])
	publicRIPEMD160 := RIPEMD160Hasher.Sum(nil)

	return publicRIPEMD160
}

const version = byte(0x00)
