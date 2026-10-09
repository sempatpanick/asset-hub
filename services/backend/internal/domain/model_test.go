package domain

import "testing"

func TestNames(t *testing.T) {
	for _, input := range []string{"", " \t ", "line\nbreak", "nul\x00byte"} {
		if _, err := CleanName(input); err == nil {
			t.Errorf("accepted invalid name %q", input)
		}
	}
	if v, err := CleanName("  Marketing  "); err != nil || v != "Marketing" {
		t.Fatal(v, err)
	}
}
func TestAdminKeyCannotBecomeAdministrator(t *testing.T) {
	p := Principal{User: User{Role: "admin"}, KeyID: "key"}
	if p.IsAdmin() {
		t.Fatal("application key gained administrator privileges")
	}
	p.KeyID = ""
	if !p.IsAdmin() {
		t.Fatal("administrator denied")
	}
	p.User.Disabled = true
	if p.IsAdmin() {
		t.Fatal("disabled administrator allowed")
	}
}
