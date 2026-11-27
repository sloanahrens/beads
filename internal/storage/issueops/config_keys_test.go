package issueops

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func configKeysMock(t *testing.T) (sqlmock.Sqlmock, DBTX) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return mock, db
}

func TestGetConfigKeysInTxTwoKeysOneQuery(t *testing.T) {
	mock, db := configKeysMock(t)
	mock.ExpectQuery("SELECT `key`, value FROM config WHERE `key` IN (?,?)").
		WithArgs("a", "b").WillReturnRows(sqlmock.NewRows([]string{"key", "value"}).AddRow("a", "1").AddRow("b", "2"))
	got, err := GetConfigKeysInTx(context.Background(), db, "a", "b")
	if err != nil {
		t.Fatalf("GetConfigKeysInTx: %v", err)
	}
	if len(got) != 2 || got["a"] != "1" || got["b"] != "2" {
		t.Errorf("got %v, want both rows", got)
	}
}

func TestGetConfigKeysInTxMissingKeyAbsent(t *testing.T) {
	mock, db := configKeysMock(t)
	mock.ExpectQuery("SELECT `key`, value FROM config WHERE `key` IN (?,?)").
		WithArgs("present", "absent").WillReturnRows(sqlmock.NewRows([]string{"key", "value"}).AddRow("present", "v"))
	got, err := GetConfigKeysInTx(context.Background(), db, "present", "absent")
	if err != nil {
		t.Fatalf("GetConfigKeysInTx: %v", err)
	}
	if len(got) != 1 || got["present"] != "v" {
		t.Errorf("got %v, want only the present key", got)
	}
}

func TestGetConfigKeysInTxNoKeysNoQuery(t *testing.T) {
	_, db := configKeysMock(t) // nothing expected: a query fails the mock
	got, err := GetConfigKeysInTx(context.Background(), db)
	if err != nil {
		t.Fatalf("GetConfigKeysInTx: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %v, want an empty map", got)
	}
}

func TestGetConfigKeysInTxQueryErrorReturned(t *testing.T) {
	mock, db := configKeysMock(t)
	wantErr := errors.New("boom")
	mock.ExpectQuery("SELECT `key`, value FROM config WHERE `key` IN (?)").WithArgs("k").WillReturnError(wantErr)
	if _, err := GetConfigKeysInTx(context.Background(), db, "k"); !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
}
