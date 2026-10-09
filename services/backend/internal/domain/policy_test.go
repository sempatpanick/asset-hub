package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestQuotaCannotShrinkBelowUsage(t *testing.T) {
	if !errors.Is(ValidateQuota(1<<30, 2<<30), ErrConflict) {
		t.Fatal("quota smaller than usage allowed")
	}
	if ValidateQuota(2<<30, 1<<30) != nil {
		t.Fatal("valid quota rejected")
	}
}
func TestLastManagerIsPreserved(t *testing.T) {
	if !errors.Is(ValidateMemberChange("manager", "viewer", 1), ErrConflict) {
		t.Fatal("last manager demoted")
	}
	if ValidateMemberChange("manager", "viewer", 2) != nil {
		t.Fatal("additional manager could not be demoted")
	}
	if ValidateMemberChange("", "viewer", 1) != nil {
		t.Fatal("new viewer rejected")
	}
}
func TestNamesUseUnicodeCharacters(t *testing.T) {
	if _, err := CleanName(strings.Repeat("界", 100)); err != nil {
		t.Fatal("valid Unicode name rejected")
	}
	if _, err := CleanName(strings.Repeat("界", 181)); !errors.Is(err, ErrInvalid) {
		t.Fatal("overlong Unicode name allowed")
	}
}
