// Copyright 2026 The Sqlite Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sqlite // import "modernc.org/sqlite"

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	sqlite3 "modernc.org/sqlite/lib"
)

type authCall struct {
	action                          int32
	arg1, arg2, database, triggerNm string
}

func openOneConn(t *testing.T) (*sql.DB, *sql.Conn) {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "main.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return db, conn
}

func registerAuthorizer(t *testing.T, conn *sql.Conn, fn AuthorizerFn) {
	t.Helper()
	if err := conn.Raw(func(dc any) error { return dc.(Authorizer).RegisterAuthorizer(fn) }); err != nil {
		t.Fatal(err)
	}
}

func TestAuthorizerSeesActionsAndCanDeny(t *testing.T) {
	ctx := context.Background()
	_, conn := openOneConn(t)
	if _, err := conn.ExecContext(ctx, "CREATE TABLE t(a)"); err != nil {
		t.Fatal(err)
	}

	var calls []authCall
	registerAuthorizer(t, conn, func(action int32, arg1, arg2, database, trigger string) int32 {
		calls = append(calls, authCall{action, arg1, arg2, database, trigger})
		if action == sqlite3.SQLITE_PRAGMA {
			return sqlite3.SQLITE_DENY
		}
		return sqlite3.SQLITE_OK
	})

	if _, err := conn.ExecContext(ctx, "INSERT INTO t VALUES (1)"); err != nil {
		t.Fatalf("INSERT under an allowing authorizer: %v", err)
	}
	want := authCall{action: sqlite3.SQLITE_INSERT, arg1: "t", database: "main"}
	if !containsCall(calls, want) {
		t.Fatalf("calls %+v carry no %+v", calls, want)
	}

	calls = nil
	_, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = ON")
	var se *Error
	if !errors.As(err, &se) || se.Code() != sqlite3.SQLITE_AUTH {
		t.Fatalf("denied PRAGMA = %v, want SQLITE_AUTH", err)
	}
	want = authCall{action: sqlite3.SQLITE_PRAGMA, arg1: "foreign_keys", arg2: "ON"}
	if !containsCall(calls, want) {
		t.Fatalf("calls %+v carry no %+v", calls, want)
	}

	registerAuthorizer(t, conn, nil)
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("PRAGMA after removing the authorizer: %v", err)
	}
}

func TestAuthorizerIsPerConnection(t *testing.T) {
	ctx := context.Background()
	_, a := openOneConn(t)
	_, b := openOneConn(t)
	registerAuthorizer(t, a, func(int32, string, string, string, string) int32 { return sqlite3.SQLITE_DENY })
	if _, err := a.ExecContext(ctx, "CREATE TABLE t(x)"); err == nil {
		t.Fatal("CREATE TABLE on the denying connection succeeded")
	}
	if _, err := b.ExecContext(ctx, "CREATE TABLE t(x)"); err != nil {
		t.Fatalf("CREATE TABLE on the other connection: %v", err)
	}
}

func TestDBConfigTogglesDefensive(t *testing.T) {
	ctx := context.Background()
	_, conn := openOneConn(t)
	dbConfig := func(value int) int {
		t.Helper()
		var got int
		if err := conn.Raw(func(dc any) error {
			var err error
			got, err = dc.(DBConfigurer).DBConfig(sqlite3.SQLITE_DBCONFIG_DEFENSIVE, value)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return got
	}
	if got := dbConfig(-1); got != 0 {
		t.Fatalf("DEFENSIVE on a fresh connection = %d, want 0", got)
	}
	if got := dbConfig(1); got != 1 {
		t.Fatalf("DEFENSIVE after setting 1 = %d", got)
	}
	if _, err := conn.ExecContext(ctx, "PRAGMA writable_schema = ON"); err != nil {
		t.Fatal(err)
	}
	var on int
	if err := conn.QueryRowContext(ctx, "PRAGMA writable_schema").Scan(&on); err != nil {
		t.Fatal(err)
	}
	if on != 0 {
		t.Fatal("writable_schema turned on under DEFENSIVE")
	}
	if got := dbConfig(0); got != 0 {
		t.Fatalf("DEFENSIVE after setting 0 = %d", got)
	}
	if _, err := conn.ExecContext(ctx, "PRAGMA writable_schema = ON"); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRowContext(ctx, "PRAGMA writable_schema").Scan(&on); err != nil {
		t.Fatal(err)
	}
	if on != 1 {
		t.Fatal("writable_schema stayed off with DEFENSIVE cleared")
	}
}

func containsCall(calls []authCall, want authCall) bool {
	for _, c := range calls {
		if c == want {
			return true
		}
	}
	return false
}
