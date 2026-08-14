// Copyright 2026 The Sqlite Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sqlite // import "modernc.org/sqlite"

import (
	sqlite3 "modernc.org/sqlite/lib"
)

// PagerIdentifier is implemented by this package's driver connections. It
// reports the address sqlite3.DirtyPagesHook labels a connection's pages with.
//
// The hook is process-global and page numbers repeat across databases, so a
// caller buffering pages per database needs both halves: the address from here,
// taken through (*sql.Conn).Raw or a connection hook, and the address the hook
// reports. Zero means the connection has no main b-tree yet.
//
// The address belongs to the connection and is freed with it. A table keyed by
// it must lose the entry when the connection closes, because a later connection
// can be handed the same address.
type PagerIdentifier interface {
	MainPager() uintptr
}

// MainPager implements PagerIdentifier.
func (c *conn) MainPager() uintptr {
	return sqlite3.MainPager(c.tls, c.db)
}
