package core

import (
	"crypto/sha256"
	"golang.org/x/crypto/ripemd160"
	"encoding/binary"
	"fmt"
	"io"
	"os"
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

func CopyDir(src string, dst string) error {
	var err error
	fds, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	err = os.MkdirAll(dst, 0755)
	if err != nil {
		return err
	}
	for _, fd := range fds {
		srcfp := src + "/" + fd.Name()
		dstfp := dst + "/" + fd.Name()
		if fd.IsDir() {
			err = CopyDir(srcfp, dstfp)
			if err != nil {
				// fmt.Println(err)
			}
		} else {
			in, err := os.Open(srcfp)
			if err != nil {
				return err
			}
			out, err := os.Create(dstfp)
			if err != nil {
				in.Close()
				return err
			}
			_, err = io.Copy(out, in)
			in.Close()
			out.Close()
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func HashPubKey(pubKey []byte) []byte {
	publicSHA256 := sha256.Sum256(pubKey)

	RIPEMD160Hasher := ripemd160.New()
	_, _ = RIPEMD160Hasher.Write(publicSHA256[:])
	publicRIPEMD160 := RIPEMD160Hasher.Sum(nil)

	return publicRIPEMD160
}
const version = byte(0x00)
