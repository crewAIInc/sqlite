// Copyright 2026 The Sqlite Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build (netbsd && amd64) || (freebsd && 386) || (freebsd && arm)

package sqlite3

import (
	"modernc.org/libc"
)

// MainPager reports no pager on the targets whose transpiled source predates
// the generator dirtypages_capture.go is written against. Those targets take no
// hook call site, so DirtyPagesHook never fires there and a caller pairing the
// two values finds nothing to pair.
//
// There is deliberately no callDirtyPagesHook here: an inserted call site on a
// target this file covers must fail to build rather than compile into a hook
// that observes nothing.
func MainPager(tls *libc.TLS, db uintptr) uintptr { return 0 }
