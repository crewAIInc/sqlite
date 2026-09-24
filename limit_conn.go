// Copyright 2026 The Sqlite Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sqlite // import "modernc.org/sqlite"

// Limiter is implemented by the driver's connection. It sets a sqlite3_limit
// value on a connection before database/sql has handed it out, which Limit
// cannot do because it needs a *sql.Conn from the pool.
//
// Use it from a driver.Connector's Connect, or from a ConnectionHookFn by
// asserting the hook's conn to Limiter. A limit set there holds for every
// connection the pool ever opens, including the replacement for one that
// database/sql discarded after an interrupted statement.
type Limiter interface {
	// Limit calls sqlite3_limit on this connection and returns the prior
	// value. A negative newVal reads the current value without changing it.
	Limit(id int, newVal int) int
}

var _ Limiter = (*conn)(nil)

// Limit implements Limiter.
func (c *conn) Limit(id int, newVal int) int {
	return c.limit(id, newVal)
}
