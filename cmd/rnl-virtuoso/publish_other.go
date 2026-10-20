//go:build !linux

package main

import "errors"

func publish(staging, output string) error {
	return errors.New("native export publishing requires Linux")
}
