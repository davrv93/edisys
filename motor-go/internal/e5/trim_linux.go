//go:build linux

package e5

/*
#include <malloc.h>
*/
import "C"

// liberarHeap devuelve al sistema la memoria libre del heap de C (glibc):
// el tokenizer y onnxruntime dejan cientos de MB de temporales tras cargar.
func liberarHeap() { C.malloc_trim(0) }
