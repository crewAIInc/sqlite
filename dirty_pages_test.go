// Copyright 2026 The Sqlite Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sqlite // import "modernc.org/sqlite"

import (
	"context"
	"database/sql"
	"path/filepath"
	"sync"
	"testing"

	sqlite3 "modernc.org/sqlite/lib"
)

type capturedPages struct {
	pager     uintptr
	pgnos     []uint32
	first     []byte
	nTruncate uint32
	isCommit  bool
}

// captureFor installs DirtyPagesHook for the duration of a test and returns the
// calls it saw. The hook is process-global, so the test cannot run in parallel
// with another that installs it.
func captureFor(t *testing.T) *[]capturedPages {
	t.Helper()
	var mu sync.Mutex
	calls := []capturedPages{}
	sqlite3.DirtyPagesHook = func(pager uintptr, pages []sqlite3.DirtyPage, nTruncate uint32, isCommit bool) {
		c := capturedPages{pager: pager, nTruncate: nTruncate, isCommit: isCommit}
		for _, p := range pages {
			c.pgnos = append(c.pgnos, p.Pgno)
		}
		if len(pages) != 0 {
			c.first = append([]byte(nil), pages[0].Data...)
		}
		mu.Lock()
		calls = append(calls, c)
		mu.Unlock()
	}
	t.Cleanup(func() { sqlite3.DirtyPagesHook = nil })
	return &calls
}

// TestDirtyPagesHookIsLive fails loudly if the call site inserted into
// _pagerWalFrames stops being reached. A capture path that observes nothing
// reports no error of its own, so this is the assertion that a regeneration
// dropping the patch breaks something.
func TestDirtyPagesHookIsLive(t *testing.T) {
	calls := captureFor(t)

	dir := t.TempDir()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "t.db")+"?_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(`CREATE TABLE t(id INTEGER PRIMARY KEY, v TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO t VALUES(1,'a')`); err != nil {
		t.Fatal(err)
	}

	if len(*calls) == 0 {
		t.Fatal("DirtyPagesHook was never called: the _pagerWalFrames call site is gone")
	}

	var commits int
	for _, c := range *calls {
		if !c.isCommit {
			continue
		}
		commits++
		if c.pager == 0 {
			t.Error("commit reported a zero pager")
		}
		if c.nTruncate == 0 {
			t.Error("commit reported nTruncate 0")
		}
		if len(c.pgnos) == 0 {
			t.Error("commit reported no pages")
		}
		if len(c.first) != 4096 {
			t.Errorf("page data length = %d, want the page size 4096", len(c.first))
		}
	}
	if commits < 2 {
		t.Errorf("commits observed = %d, want at least 2", commits)
	}
}

// TestMainPagerMatchesHook pins the pairing the hook's caller depends on: the
// address a connection reports is the address its pages arrive under.
func TestMainPagerMatchesHook(t *testing.T) {
	calls := captureFor(t)

	dir := t.TempDir()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "t.db")+"?_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	ctx := context.Background()
	sqlConn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer sqlConn.Close()

	var pager uintptr
	if err := sqlConn.Raw(func(dc any) error {
		pi, ok := dc.(PagerIdentifier)
		if !ok {
			t.Fatalf("driver conn %T does not implement PagerIdentifier", dc)
		}
		pager = pi.MainPager()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if pager == 0 {
		t.Fatal("MainPager returned 0 on an open connection")
	}

	if _, err := sqlConn.ExecContext(ctx, `CREATE TABLE t(id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if len(*calls) == 0 {
		t.Fatal("DirtyPagesHook was never called")
	}
	for _, c := range *calls {
		if c.pager != pager {
			t.Fatalf("hook reported pager %#x, MainPager reported %#x", c.pager, pager)
		}
	}
}

// TestMainPagerOnSeparateDatabases covers the reason the hook carries a pager
// at all: two databases in one process report different addresses, and page
// numbers repeat across them.
func TestMainPagerOnSeparateDatabases(t *testing.T) {
	calls := captureFor(t)

	dir := t.TempDir()
	pagers := make([]uintptr, 2)
	for i, name := range []string{"a.db", "b.db"} {
		db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, name)+"?_pragma=journal_mode(WAL)")
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		db.SetMaxOpenConns(1)
		if _, err := db.Exec(`CREATE TABLE t(id INTEGER PRIMARY KEY)`); err != nil {
			t.Fatal(err)
		}
		sqlConn, err := db.Conn(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		defer sqlConn.Close()
		if err := sqlConn.Raw(func(dc any) error {
			pagers[i] = dc.(PagerIdentifier).MainPager()
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	if pagers[0] == 0 || pagers[1] == 0 {
		t.Fatalf("MainPager returned 0: %#x %#x", pagers[0], pagers[1])
	}
	if pagers[0] == pagers[1] {
		t.Fatalf("two databases reported the same pager %#x", pagers[0])
	}

	seen := map[uintptr]bool{}
	for _, c := range *calls {
		seen[c.pager] = true
	}
	if len(seen) != 2 {
		t.Fatalf("hook reported %d distinct pagers, want 2", len(seen))
	}
}
