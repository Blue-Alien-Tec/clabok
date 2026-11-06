package asciiui

import "testing"

func TestValidate(t *testing.T) {
	if err := Validate("legend true\nbutton Buy\n"); err != nil {
		t.Fatalf("valid spec: %v", err)
	}
	if err := Validate("legend false\n"); err == nil {
		t.Fatal("a spec without nodes must be rejected")
	}
	if err := Validate("bogus x\n"); err == nil {
		t.Fatal("an unknown widget must be rejected")
	}
}
