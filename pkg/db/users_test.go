package db

import (
	"testing"
	"time"
)

func TestRegisterUserAndSession(t *testing.T) {
	database := openTestDB(t)

	user, err := database.CreateUser("Anna@Mail.Ru", "secret123", "Anna")
	if err != nil {
		t.Fatal(err)
	}
	if user.Email != "anna@mail.ru" {
		t.Fatalf("email normalize: %q", user.Email)
	}
	if _, err := database.CreateUser("anna@mail.ru", "secret123", "Anna"); err == nil {
		t.Fatal("expected duplicate email error")
	}

	authed, err := database.AuthenticateUser("anna@mail.ru", "secret123")
	if err != nil || authed.ID != user.ID {
		t.Fatalf("auth: %v %+v", err, authed)
	}

	session, err := database.CreateSession(user.ID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	got, err := database.UserBySession(session.Token)
	if err != nil || got.Email != user.Email {
		t.Fatalf("session user: %v %+v", err, got)
	}
}
