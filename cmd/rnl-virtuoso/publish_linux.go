package main

import (
	"os"
	"syscall"
)

// Reserve the destination exclusively. Linux rename(2) can atomically replace
// our empty reservation with the complete staging directory, whereas os.Rename
// deliberately rejects directory destinations even on Linux. A concurrent file
// inside the reservation makes rename fail; cleanup never removes user contents.
func publish(staging, output string) error {
	if e := os.Mkdir(output, 0755); e != nil {
		return e
	}
	if e := syscall.Rename(staging, output); e != nil {
		_ = os.Remove(output)
		return e
	}
	return nil
}
