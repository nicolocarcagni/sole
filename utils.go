package main

import (
	"bytes"
	"encoding/binary"
	"io"
	"log"
	"os"
)

// ExtractPubKeyHash decodes a Base58 address and strips the version and checksum.
func ExtractPubKeyHash(address string) ([]byte, error) {
	pubKeyHash, err := Base58Decode([]byte(address))
	if err != nil {
		return nil, err
	}
	if len(pubKeyHash) < 5 {
		return nil, log.Output(2, "Invalid address length")
	}
	return pubKeyHash[1 : len(pubKeyHash)-4], nil
}

// AddressFromPubKeyHash takes a PubKeyHash and returns a Base58 encoded address.
func AddressFromPubKeyHash(pubKeyHash []byte) string {
	versionedPayload := append([]byte{version}, pubKeyHash...)
	checksum := checksum(versionedPayload)
	fullPayload := append(versionedPayload, checksum...)
	return string(Base58Encode(fullPayload))
}


// IntToHex converts an int64 to a byte array
func IntToHex(num int64) []byte {
	buff := new(bytes.Buffer)
	err := binary.Write(buff, binary.BigEndian, num)
	if err != nil {
		log.Panic(err)
	}
	return buff.Bytes()
}

// CopyDir copies a directory recursively
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
