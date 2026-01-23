package main

import (
	"bytes"
	"encoding/binary"
	"io"
	"log"
	"os"
)

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
