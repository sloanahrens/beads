package dolt

import (
	"context"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func guardCfg() *Config {
	return &Config{Database: "scratch", ServerHost: "127.0.0.1", ServerPort: 3307}
}

func TestRefuseNewDatabaseOnPopulatedServer(t *testing.T) {
	tests := []struct {
		name      string
		databases []string
		wantErr   bool
	}{
		{"fresh server has only system databases", []string{"information_schema", "mysql"}, false},
		{"server with another user database", []string{"information_schema", "mysql", "beads_gt"}, true},
		{"only the target already listed", []string{"information_schema", "scratch"}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			rows := sqlmock.NewRows([]string{"Database"})
			for _, n := range tc.databases {
				rows.AddRow(n)
			}
			mock.ExpectQuery("SHOW DATABASES").WillReturnRows(rows)

			err = refuseNewDatabaseOnPopulatedServer(context.Background(), db, guardCfg())
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if err != nil {
				for _, want := range []string{"--allow-new-database", AllowNewDatabaseEnv + "=1", "beads_gt", "127.0.0.1:3307"} {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error missing %q: %v", want, err)
					}
				}
			}
		})
	}
}

func TestNewDatabaseRefusalTruncatesLongList(t *testing.T) {
	others := []string{"a", "b", "c", "d", "e", "f", "g"}
	msg := newDatabaseRefusal(guardCfg(), others).Error()
	if !strings.Contains(msg, "7 other database(s)") || !strings.Contains(msg, "and 2 more") || strings.Contains(msg, ", f") {
		t.Errorf("unexpected message: %s", msg)
	}
}
