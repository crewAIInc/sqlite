// Copyright 2026 The Sqlite Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sqlite // import "modernc.org/sqlite"

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"path/filepath"
	"strings"
	"testing"
)

// testLimitAttached is SQLITE_LIMIT_ATTACHED from sqlite3.h.
const testLimitAttached = 7

// limitConnector opens every connection through the driver and sets
// SQLITE_LIMIT_ATTACHED=0 on it before database/sql sees it.
type limitConnector struct{ dsn string }

func (c limitConnector) Connect(context.Context) (driver.Conn, error) {
	dc, err := newDriver().Open(c.dsn)
	if err != nil {
		return nil, err
	}
	dc.(Limiter).Limit(testLimitAttached, 0)
	return dc, nil
}

func (limitConnector) Driver() driver.Driver { return newDriver() }

func TestLimiterAppliesToEveryPooledConnection(t *testing.T) {
	dir := t.TempDir()
	db := sql.OpenDB(limitConnector{dsn: filepath.Join(dir, "main.db")})
	defer db.Close()
	db.SetMaxOpenConns(1)

	attach := "ATTACH DATABASE '" + filepath.Join(dir, "other.db") + "' AS other"
	for i := 0; i < 2; i++ {
		if _, err := db.Exec(attach); err == nil || !strings.Contains(err.Error(), "too many attached") {
			t.Fatalf("round %d: ATTACH = %v, want the attach limit's refusal", i, err)
		}
		// Discard the pooled connection, so the next round runs on one
		// the pool opened fresh.
		conn, err := db.Conn(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if err := conn.Raw(func(any) error { return driver.ErrBadConn }); err == nil {
			t.Fatal("Raw returned nil, want ErrBadConn")
		}
		_ = conn.Close()
		if n := db.Stats().OpenConnections; n != 0 {
			t.Fatalf("round %d: %d connections still open after the discard", i, n)
		}
	}
}

func TestLimiterReturnsPriorValue(t *testing.T) {
	dc, err := newDriver().Open(filepath.Join(t.TempDir(), "main.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer dc.Close()
	l := dc.(Limiter)
	if prev := l.Limit(testLimitAttached, 0); prev <= 0 {
		t.Fatalf("prior SQLITE_LIMIT_ATTACHED = %d, want SQLite's positive default", prev)
	}
	if got := l.Limit(testLimitAttached, -1); got != 0 {
		t.Fatalf("SQLITE_LIMIT_ATTACHED after setting 0 = %d", got)
	}
}
