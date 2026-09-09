package doltserver

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestSelectOrphanTestServerPIDs pins down the safety-critical selection
// logic used by SweepOrphanedTestServers: only cmdlines that look like a
// dolt sql-server are candidates at all, and a *live* (non-deleted-cwd) one
// is only reaped when its cwd is nested under a root the caller explicitly
// vouches for as its own suite — never merely because it sits somewhere
// under a shared/global temp dir. That distinction is the whole point: a
// parallel test run (scripts/test.sh -p N) has many suites with live
// servers all living under os.TempDir(), and only a suite's own scoped
// root may reap its own debris, not everyone else's (gastownhall/beads
// mybd-q6cz).
func TestSelectOrphanTestServerPIDs(t *testing.T) {
	cases := []struct {
		name       string
		candidates []serverCandidate
		suiteRoots []string
		want       []int
	}{
		{
			name: "deleted cwd is reaped even with no suite roots at all",
			candidates: []serverCandidate{
				{pid: 100, cmdline: "dolt sql-server -H 127.0.0.1 -P 12345", cwd: "/tmp/beads-bd-tests-xyz/.beads/dolt", cwdDeleted: true},
			},
			suiteRoots: nil,
			want:       []int{100},
		},
		{
			name: "live server under the caller's own scoped root is reaped",
			candidates: []serverCandidate{
				{pid: 101, cmdline: "dolt sql-server -H 127.0.0.1 -P 12345", cwd: "/tmp/my-suite-root/.beads/dolt"},
			},
			suiteRoots: []string{"/tmp/my-suite-root"},
			want:       []int{101},
		},
		{
			name: "live server under a DIFFERENT suite's temp dir is NOT reaped, even though both sit under the same global temp dir",
			candidates: []serverCandidate{
				{pid: 102, cmdline: "dolt sql-server -H 127.0.0.1 -P 12345", cwd: "/tmp/other-suite-xyz/.beads/dolt"},
			},
			suiteRoots: []string{"/tmp/my-suite-root"},
			want:       nil,
		},
		{
			name: "debug-mode cmdline with flags before sql-server is still matched when under the scoped root",
			candidates: []serverCandidate{
				{pid: 103, cmdline: "dolt --prof cpu --prof-path /tmp/my-suite-root/dolt-pprof sql-server -H 127.0.0.1 -P 12345", cwd: "/tmp/my-suite-root/.beads/dolt"},
			},
			suiteRoots: []string{"/tmp/my-suite-root"},
			want:       []int{103},
		},
		{
			name: "production server outside any suite root is never reaped",
			candidates: []serverCandidate{
				{pid: 200, cmdline: "dolt sql-server -H 127.0.0.1 -P 3307", cwd: "/home/dev/project/.beads/dolt"},
			},
			suiteRoots: []string{"/tmp/my-suite-root"},
			want:       nil,
		},
		{
			name: "non-dolt process under the scoped root is ignored",
			candidates: []serverCandidate{
				{pid: 201, cmdline: "some-other-binary --flag", cwd: "/tmp/my-suite-root/whatever"},
			},
			suiteRoots: []string{"/tmp/my-suite-root"},
			want:       nil,
		},
		{
			name: "dolt process without sql-server subcommand is ignored",
			candidates: []serverCandidate{
				{pid: 202, cmdline: "dolt status", cwd: "/tmp/my-suite-root/whatever"},
			},
			suiteRoots: []string{"/tmp/my-suite-root"},
			want:       nil,
		},
		{
			name: "empty cwd and not deleted is left alone (unknown, not provably debris)",
			candidates: []serverCandidate{
				{pid: 203, cmdline: "dolt sql-server -H 127.0.0.1 -P 12345", cwd: ""},
			},
			suiteRoots: []string{"/tmp/my-suite-root"},
			want:       nil,
		},
		{
			name: "scoped-root sibling path is not treated as under the root",
			candidates: []serverCandidate{
				{pid: 204, cmdline: "dolt sql-server -H 127.0.0.1 -P 12345", cwd: "/tmp/my-suite-root2/evil"},
			},
			suiteRoots: []string{"/tmp/my-suite-root"},
			want:       nil,
		},
		{
			name: "no suite roots configured: only the deleted-cwd signal reaps anything",
			candidates: []serverCandidate{
				{pid: 205, cmdline: "dolt sql-server -H 127.0.0.1 -P 12345", cwd: "/tmp/some-suite/.beads/dolt"},
			},
			suiteRoots: nil,
			want:       nil,
		},
		{
			name: "mixed candidates: production dir, another suite's live server, this suite's live server, and a deleted-cwd orphan",
			candidates: []serverCandidate{
				{pid: 300, cmdline: "dolt sql-server -P 1", cwd: "/home/dev/real-project/.beads/dolt"},
				{pid: 301, cmdline: "dolt sql-server -P 2", cwd: "/tmp/other-suite-abc/.beads/dolt"},
				{pid: 302, cmdline: "dolt sql-server -P 3", cwd: "/tmp/my-suite-root/.beads/dolt"},
				{pid: 303, cmdline: "dolt sql-server -P 4", cwdDeleted: true, cwd: "/tmp/whatever-else/.beads/dolt"},
			},
			suiteRoots: []string{"/tmp/my-suite-root"},
			want:       []int{302, 303},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := selectOrphanTestServerPIDs(tc.candidates, tc.suiteRoots)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("selectOrphanTestServerPIDs() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestSelectDeadOwnerServerPIDs pins down the owner-marker selection path
// (be-4c2): a candidate is only reaped when IT ITSELF recorded an owner PID
// and that specific PID is confirmed dead — never merely because it sits
// under some directory naming convention. This is what lets a server whose
// owning test process was SIGKILLed get reaped on a later, unrelated run's
// startup sweep, without the naming-convention regression that
// TestSelectOrphanTestServerPIDs guards against (gastownhall/beads
// mybd-q6cz): a live parallel suite's server names its own (live) owner PID,
// so isProcessAlive keeps it safe regardless of where its data dir lives.
func TestSelectDeadOwnerServerPIDs(t *testing.T) {
	alive := map[int]bool{111: true, 222: false}
	isProcessAlive := func(pid int) bool { return alive[pid] }

	cases := []struct {
		name       string
		candidates []serverCandidate
		want       []int
	}{
		{
			name: "dead owner is reaped",
			candidates: []serverCandidate{
				{pid: 1, cmdline: "dolt sql-server -P 1", cwd: "/tmp/whatever", ownerPID: 222},
			},
			want: []int{1},
		},
		{
			name: "live owner is left alone",
			candidates: []serverCandidate{
				{pid: 2, cmdline: "dolt sql-server -P 2", cwd: "/tmp/whatever", ownerPID: 111},
			},
			want: nil,
		},
		{
			name: "no recorded owner (production server) is left alone",
			candidates: []serverCandidate{
				{pid: 3, cmdline: "dolt sql-server -P 3", cwd: "/home/dev/project/.beads/dolt"},
			},
			want: nil,
		},
		{
			name: "non-dolt process with a dead owner PID is ignored",
			candidates: []serverCandidate{
				{pid: 4, cmdline: "some-other-binary", cwd: "/tmp/whatever", ownerPID: 222},
			},
			want: nil,
		},
		{
			name: "mixed: dead-owner orphan alongside a live-owner and a no-owner server",
			candidates: []serverCandidate{
				{pid: 5, cmdline: "dolt sql-server -P 5", cwd: "/tmp/a", ownerPID: 222},
				{pid: 6, cmdline: "dolt sql-server -P 6", cwd: "/tmp/b", ownerPID: 111},
				{pid: 7, cmdline: "dolt sql-server -P 7", cwd: "/home/dev/real/.beads/dolt"},
			},
			want: []int{5},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := selectDeadOwnerServerPIDs(tc.candidates, isProcessAlive)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("selectDeadOwnerServerPIDs() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestReadTestOwnerPID(t *testing.T) {
	t.Run("valid pid file", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, TestOwnerPIDFileName), []byte("1234\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if got := readTestOwnerPID(dir); got != 1234 {
			t.Errorf("readTestOwnerPID() = %d, want 1234", got)
		}
	})

	t.Run("missing file returns 0", func(t *testing.T) {
		if got := readTestOwnerPID(t.TempDir()); got != 0 {
			t.Errorf("readTestOwnerPID() = %d, want 0", got)
		}
	})

	t.Run("empty cwd returns 0", func(t *testing.T) {
		if got := readTestOwnerPID(""); got != 0 {
			t.Errorf("readTestOwnerPID() = %d, want 0", got)
		}
	})

	t.Run("garbage contents returns 0", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, TestOwnerPIDFileName), []byte("not-a-pid"), 0600); err != nil {
			t.Fatal(err)
		}
		if got := readTestOwnerPID(dir); got != 0 {
			t.Errorf("readTestOwnerPID() = %d, want 0", got)
		}
	})
}

func TestCountOrphanCandidates(t *testing.T) {
	alive := map[int]bool{111: true}
	isProcessAlive := func(pid int) bool { return alive[pid] }

	candidates := []serverCandidate{
		{pid: 1, cmdline: "dolt sql-server -P 1", cwdDeleted: true, cwd: "/tmp/deleted/.beads/dolt"},
		{pid: 2, cmdline: "dolt sql-server -P 2", cwd: "/tmp/dead-owner/.beads/dolt", ownerPID: 222},
		{pid: 3, cmdline: "dolt sql-server -P 3", cwd: "/tmp/live-owner/.beads/dolt", ownerPID: 111},
		{pid: 4, cmdline: "dolt sql-server -P 4", cwd: "/home/dev/real-project/.beads/dolt"},
	}

	if got := countOrphanCandidates(candidates, isProcessAlive); got != 2 {
		t.Errorf("countOrphanCandidates() = %d, want 2", got)
	}
}

func TestMergePIDs(t *testing.T) {
	cases := []struct {
		name string
		a, b []int
		want []int
	}{
		{name: "b empty returns a", a: []int{1, 2}, b: nil, want: []int{1, 2}},
		{name: "disjoint appends b after a", a: []int{1}, b: []int{2}, want: []int{1, 2}},
		{name: "overlap is deduped", a: []int{1, 2}, b: []int{2, 3}, want: []int{1, 2, 3}},
		{name: "both empty", a: nil, b: nil, want: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mergePIDs(tc.a, tc.b)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("mergePIDs(%v, %v) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

func TestIsUnderDir(t *testing.T) {
	cases := []struct {
		dir, root string
		want      bool
	}{
		{"/tmp", "/tmp", true},
		{"/tmp/foo", "/tmp", true},
		{"/tmp/foo/bar", "/tmp", true},
		{"/tmp2/foo", "/tmp", false},
		{"/tmpfoo", "/tmp", false},
		{"/", "/tmp", false},
		{"/tmp/foo", "", false},
	}
	for _, tc := range cases {
		if got := isUnderDir(tc.dir, tc.root); got != tc.want {
			t.Errorf("isUnderDir(%q, %q) = %v, want %v", tc.dir, tc.root, got, tc.want)
		}
	}
}

func TestGatherPSCandidates(t *testing.T) {
	psOutput := []byte(`
  101 dolt sql-server -H 127.0.0.1 -P 12345
not-a-pid dolt sql-server
  102 dolt status
  103 /opt/dolt --prof cpu sql-server -P 12346
  104 dolt sql-server -P 12347
`)
	cwds := map[int]struct {
		dir     string
		deleted bool
		ok      bool
	}{
		101: {dir: "/tmp/my-suite/.beads/dolt", ok: true},
		103: {dir: "/tmp/deleted-suite/.beads/dolt", deleted: true, ok: true},
		104: {ok: false},
	}

	candidates := gatherPSCandidates(psOutput, func(pid int) (string, bool, bool) {
		cwd := cwds[pid]
		return cwd.dir, cwd.deleted, cwd.ok
	})
	wantCandidates := []serverCandidate{
		{pid: 101, cmdline: "dolt sql-server -H 127.0.0.1 -P 12345", cwd: "/tmp/my-suite/.beads/dolt"},
		{pid: 103, cmdline: "/opt/dolt --prof cpu sql-server -P 12346", cwd: "/tmp/deleted-suite/.beads/dolt", cwdDeleted: true},
	}
	if !reflect.DeepEqual(candidates, wantCandidates) {
		t.Fatalf("gatherPSCandidates() = %#v, want %#v", candidates, wantCandidates)
	}

	gotPIDs := selectOrphanTestServerPIDs(candidates, []string{"/tmp/my-suite"})
	wantPIDs := []int{101, 103}
	if !reflect.DeepEqual(gotPIDs, wantPIDs) {
		t.Errorf("darwin ps selection path = %v, want %v", gotPIDs, wantPIDs)
	}
}
