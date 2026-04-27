package identity

import "testing"

func TestRegisterCredentialsValidateNormalizesEmail(t *testing.T) {
	credentials := RegisterCredentials{
		Email:           " USER@Example.com ",
		Password:        "StrongPassw0rd!",
		PasswordConfirm: "StrongPassw0rd!",
	}

	normalized, err := credentials.Validate()
	if err != nil {
		t.Fatalf("validate returned error: %v", err)
	}

	if normalized != "user@example.com" {
		t.Fatalf("expected normalized email user@example.com, got %q", normalized)
	}
}

func TestRegisterCredentialsValidateRejectsPasswordConfirmMismatch(t *testing.T) {
	credentials := RegisterCredentials{
		Email:           "user@example.com",
		Password:        "StrongPassw0rd!",
		PasswordConfirm: "AnotherPassw0rd!",
	}

	_, err := credentials.Validate()
	if err != ErrPasswordConfirmMismatch {
		t.Fatalf("expected ErrPasswordConfirmMismatch, got %v", err)
	}
}

func TestLoginCredentialsValidateRejectsInvalidEmail(t *testing.T) {
	credentials := LoginCredentials{
		Email:    "not-an-email",
		Password: "StrongPassw0rd!",
	}

	_, err := credentials.Validate()
	if err != ErrInvalidEmail {
		t.Fatalf("expected ErrInvalidEmail, got %v", err)
	}
}
