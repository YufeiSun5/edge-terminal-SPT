package main

import "testing"

func TestDisplayUser(t *testing.T) {
	if displayUser("") != "<empty>" || displayUser("   ") != "<empty>" {
		t.Fatal("expected empty username label")
	}
	if displayUser("Admin") != "Admin" {
		t.Fatal("expected username passthrough")
	}
}
