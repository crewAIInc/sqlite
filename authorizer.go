// Copyright 2026 The Sqlite Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sqlite // import "modernc.org/sqlite"

import (
	"fmt"
	"sync"
	"unsafe"

	"modernc.org/libc"
	"modernc.org/libc/sys/types"
	sqlite3 "modernc.org/sqlite/lib"
)

// AuthorizerFn decides one sqlite3_set_authorizer callback. action is one of
// the SQLITE_* action codes (SQLITE_INSERT, SQLITE_PRAGMA, ...), and arg1,
// arg2, database and trigger are the four strings SQLite passes for it, with
// a NULL reported as "". Return SQLITE_OK, SQLITE_IGNORE or SQLITE_DENY.
//
// SQLite calls it while a statement is being prepared, including a re-prepare
// after a schema change, on the goroutine running that prepare. It must not
// use the connection it is registered on.
type AuthorizerFn func(action int32, arg1, arg2, database, trigger string) int32

// Authorizer is implemented by this package's driver connections. Reach it
// through (*sql.Conn).Raw or a driver.Connector.
type Authorizer interface {
	// RegisterAuthorizer installs fn as the connection's authorizer, or
	// removes the authorizer when fn is nil. SQLite expires every statement
	// already prepared on the connection, so each one is re-prepared and
	// re-authorized before it next runs.
	RegisterAuthorizer(fn AuthorizerFn) error
}

// DBConfigurer is implemented by this package's driver connections.
type DBConfigurer interface {
	// DBConfig calls sqlite3_db_config for one of the boolean options, such as
	// SQLITE_DBCONFIG_DEFENSIVE, and returns the option's value afterwards. A
	// negative value reads the option without changing it.
	DBConfig(op int, value int) (int, error)
}

var (
	_ Authorizer   = (*conn)(nil)
	_ DBConfigurer = (*conn)(nil)
)

var xAuthorizers = struct {
	mu sync.RWMutex
	m  map[uintptr]AuthorizerFn
}{
	m: make(map[uintptr]AuthorizerFn),
}

// RegisterAuthorizer implements Authorizer.
func (c *conn) RegisterAuthorizer(fn AuthorizerFn) error {
	if fn == nil {
		rc := sqlite3.Xsqlite3_set_authorizer(c.tls, c.db, 0, 0)
		xAuthorizers.mu.Lock()
		delete(xAuthorizers.m, c.db)
		xAuthorizers.mu.Unlock()
		if rc != sqlite3.SQLITE_OK {
			return c.errstr(rc)
		}
		return nil
	}
	xAuthorizers.mu.Lock()
	xAuthorizers.m[c.db] = fn
	xAuthorizers.mu.Unlock()
	if rc := sqlite3.Xsqlite3_set_authorizer(c.tls, c.db, cFuncPointer(authorizerTrampoline), c.db); rc != sqlite3.SQLITE_OK {
		xAuthorizers.mu.Lock()
		delete(xAuthorizers.m, c.db)
		xAuthorizers.mu.Unlock()
		return c.errstr(rc)
	}
	return nil
}

// forgetAuthorizer drops the Go callback registered for db. Call it when the
// connection closes, so a later connection handed the same address starts
// with no entry.
func forgetAuthorizer(db uintptr) {
	xAuthorizers.mu.Lock()
	delete(xAuthorizers.m, db)
	xAuthorizers.mu.Unlock()
}

func authorizerTrampoline(tls *libc.TLS, handle uintptr, action int32, z1, z2, z3, z4 uintptr) int32 {
	xAuthorizers.mu.RLock()
	fn := xAuthorizers.m[handle]
	xAuthorizers.mu.RUnlock()
	if fn == nil {
		return sqlite3.SQLITE_DENY
	}
	return fn(action, libc.GoString(z1), libc.GoString(z2), libc.GoString(z3), libc.GoString(z4))
}

// DBConfig implements DBConfigurer.
//
// The vararg payload is one int and one int pointer. libc.VaList packs every
// argument into an 8-byte slot, and the int SQLite writes the result to sits
// in the same allocation after the two slots.
func (c *conn) DBConfig(op int, value int) (int, error) {
	const vaSlot = 8
	bp := libc.Xmalloc(c.tls, types.Size_t(3*vaSlot))
	if bp == 0 {
		return 0, fmt.Errorf("sqlite3_db_config(%d): out of memory", op)
	}
	defer libc.Xfree(c.tls, bp)
	pRes := bp + 2*vaSlot
	*(*int32)(unsafe.Pointer(pRes)) = 0
	if rc := sqlite3.Xsqlite3_db_config(c.tls, c.db, int32(op), libc.VaList(bp, int32(value), pRes)); rc != sqlite3.SQLITE_OK {
		return 0, fmt.Errorf("sqlite3_db_config(%d, %d): %w", op, value, c.errstr(rc))
	}
	return int(*(*int32)(unsafe.Pointer(pRes))), nil
}
