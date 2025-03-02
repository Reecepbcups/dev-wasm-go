// Code generated by wit-bindgen-go. DO NOT EDIT.

// Package stderr represents the imported interface "wasi:cli/stderr@0.2.0".
package stderr

import (
	"github.com/dev-wasm/dev-wasm-go/lib/wasi/io/streams"
	"go.bytecodealliance.org/cm"
)

// OutputStream represents the imported type alias "wasi:cli/stderr@0.2.0#output-stream".
//
// See [streams.OutputStream] for more information.
type OutputStream = streams.OutputStream

// GetStderr represents the imported function "get-stderr".
//
//	get-stderr: func() -> output-stream
//
//go:nosplit
func GetStderr() (result OutputStream) {
	result0 := wasmimport_GetStderr()
	result = cm.Reinterpret[OutputStream]((uint32)(result0))
	return
}
