package iotfwdrv

import (
	"testing"
)

func TestEncodeAndDecode(t *testing.T) {
	c := Packet{
		Name: "TEST",
		Args: map[string]interface{}{
			"a": "a value",
			"b": "b value",
			"c": "value",
		},
	}

	s := Encode(c)
	c2, err := Decode(s)
	if err != nil {
		t.Fatal(err)
	}

	if c.Name != c2.Name {
		t.Fatal("mismatch")
	}
	if len(c.Args) != len(c2.Args) {
		t.Fatal("mismatch arg count")
	}
	for k, v := range c.Args {
		if _, ok := c2.Args[k]; !ok {
			t.Fatal("missing key")
		}
		if c2.Args[k] != v {
			t.Fatal("value mismatch")
		}
	}
}
