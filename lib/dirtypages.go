// Copyright 2026 The Sqlite Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Dirty-page capture.
//
// _pagerWalFrames is where the pager hands the WAL layer the pages a
// transaction dirtied. A hook there sees the page numbers and the page bytes
// for a transaction, which is what a physical-page replication format ships.
// isCommit tells a commit apart from a page-cache spill, so a caller does not
// have to infer the transaction boundary, and nTruncate is the database size in
// pages the commit settles on.
//
// This file and dirtypages_capture.go are hand-written and are not regenerated
// by ccgo. They keep the change to the per-target transpiled files down to one
// inserted call.

package sqlite3

// DirtyPage is one page a transaction dirtied.
//
// Data aliases SQLite's own page buffer, which the pager reuses. A callback
// that keeps the bytes past its own return must copy them.
type DirtyPage struct {
	Pgno uint32
	Data []byte
}

// DirtyPagesHook, when non-nil, is called with the pages a transaction dirtied,
// immediately before the WAL layer writes them.
//
// pager identifies the database the pages belong to: the hook is
// process-global, page numbers repeat across databases, and pager is the only
// argument that separates them. MainPager returns the same value for an open
// connection, which is how a caller learns which database a callback belongs
// to.
//
// isCommit reports whether this write commits the transaction. A spill carries
// false and nTruncate zero; a commit carries true and the database size in
// pages after the transaction. Pages arrive across several calls when the page
// cache spills, and the same page number may arrive more than once, latest
// content last.
//
// The hook runs on the goroutine executing the commit, inside the pager's write
// path, and blocks it.
var DirtyPagesHook func(pager uintptr, pages []DirtyPage, nTruncate uint32, isCommit bool)
