package grpc

import "testing"

func TestDenierToProto(t *testing.T) {
	if denierToProto(nil) != 0 {
		t.Fatal("nil must map to 0")
	}
	v := 40.0
	if denierToProto(&v) != 40 {
		t.Fatal("value must pass through")
	}
}
