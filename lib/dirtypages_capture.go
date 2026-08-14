// Copyright 2026 The Sqlite Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The build constraint names the three targets whose transpiled source predates
// the generator that produces _pagerWalFrames: they carry pagerWalFrames at
// SQLite 3.41.2, 3.41.2 and 3.40.0, along with the older type names this file
// uses. They take no inserted call site, so they need no hook body either.
//
// Drift is a build failure rather than a silent loss of capture: a target that
// gains the anchor and stays excluded here fails to compile the inserted call.

//go:build !(netbsd && amd64) && !(freebsd && 386) && !(freebsd && arm)

package sqlite3

import (
	"unsafe"

	"modernc.org/libc"
)

// callDirtyPagesHook is the whole of the inserted call site's work. Keeping it
// here means the per-target patch is one line, which is what lets it be
// re-applied after a regeneration rather than rebased.
func callDirtyPagesHook(tls *libc.TLS, pPager uintptr, pList uintptr, nTruncate TPgno, isCommit int32) {
	h := DirtyPagesHook
	if h == nil || pPager == 0 || pList == 0 {
		return
	}
	sz := int((*TPager)(unsafe.Pointer(pPager)).FpageSize)
	if sz <= 0 {
		return
	}
	var pages []DirtyPage
	for p := pList; p != 0; p = (*TPgHdr)(unsafe.Pointer(p)).FpDirty {
		hdr := (*TPgHdr)(unsafe.Pointer(p))
		if hdr.FpData == 0 {
			continue
		}
		pages = append(pages, DirtyPage{
			Pgno: uint32(hdr.Fpgno),
			Data: (*libc.RawMem)(unsafe.Pointer(hdr.FpData))[:sz:sz],
		})
	}
	h(pPager, pages, uint32(nTruncate), isCommit != 0)
}

// MainPager returns the pager backing the main schema of an open connection,
// the value DirtyPagesHook reports its pages under. It returns zero if the
// connection has no main b-tree, which is the case only before the database is
// opened.
//
// The value is stable for the connection's lifetime and is freed with it, so a
// caller that keys a table by it must drop the entry when the connection
// closes: a later connection can be handed the same address.
func MainPager(tls *libc.TLS, db uintptr) uintptr {
	if db == 0 {
		return 0
	}
	aDb := (*Tsqlite3)(unsafe.Pointer(db)).FaDb
	if aDb == 0 {
		return 0
	}
	pBt := (*TDb)(unsafe.Pointer(aDb)).FpBt
	if pBt == 0 {
		return 0
	}
	return _sqlite3BtreePager(tls, pBt)
}
