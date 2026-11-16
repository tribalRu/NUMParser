package utils

import (
	"io"
	"os"
	"runtime"
	"runtime/debug"
)

func Abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func Filter[T any](arr []T, fn func(i int, e T) bool) []T {
	list := make([]T, 0, len(arr))
	for i, t := range arr {
		if !fn(i, t) {
			list = append(list, t)
		}
	}
	return list
}

func FreeOSMemGC() {
	runtime.GC()
	debug.FreeOSMemory()
}

func CopyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return err
	}

	return destFile.Sync()
}
