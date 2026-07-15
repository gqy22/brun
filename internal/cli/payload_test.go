package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestWaitForPayloadRelease(t *testing.T) {
	if err := waitForPayloadRelease(bytes.NewReader([]byte{1})); err != nil {
		t.Fatal(err)
	}
	for _, input := range [][]byte{nil, {0}} {
		if err := waitForPayloadRelease(bytes.NewReader(input)); err == nil {
			t.Fatalf("waitForPayloadRelease(%v) succeeded", input)
		}
	}
	if err := waitForPayloadRelease(strings.NewReader("x")); err == nil {
		t.Fatal("non-release byte accepted")
	}
}
