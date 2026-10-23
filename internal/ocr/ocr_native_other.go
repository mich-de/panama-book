//go:build !windows

package ocr

import (
	"context"
	"errors"
)

var errNoNative = errors.New("backend nativo disponibile solo su Windows")

func nativeProbe() error { return errNoNative }

func recognizeNative(context.Context, []string) ([]*Result, error) { return nil, errNoNative }
