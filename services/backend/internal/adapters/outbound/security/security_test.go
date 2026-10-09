package security

import "testing"

func TestPasswordVerification(t *testing.T) {
	s := Security{}
	h, err := s.HashPassword("a-long-test-password")
	if err != nil {
		t.Fatal(err)
	}
	if !s.VerifyPassword(h, "a-long-test-password") || s.VerifyPassword(h, "different-password") || s.VerifyPassword("$argon2id$v=19$m=999999999,t=999,p=2$a$b", "x") {
		t.Fatal("password verification failed")
	}
	if s.HashToken("secret") == "secret" || len(s.Token()) != 64 {
		t.Fatal("secret handling failed")
	}
}
