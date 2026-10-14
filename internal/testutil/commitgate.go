package testutil

import "sync"

// CommitGate lets a writer that lost a serialization conflict wait for the
// commit that beat it, instead of sleeping a guessed backoff. Every writer
// calls Next before an attempt and Landed after a successful commit; a loser
// blocks on the channel Next returned. Because that channel is taken before
// the attempt starts, a commit that lands while the attempt is still running
// has already closed it, so a wakeup is never lost.
type CommitGate struct {
	mu sync.Mutex
	ch chan struct{}
}

// NewCommitGate returns a gate with no commit landed yet.
func NewCommitGate() *CommitGate {
	return &CommitGate{ch: make(chan struct{})}
}

// Next returns a channel that is closed when the next commit lands. Call it
// before starting the attempt it guards.
func (g *CommitGate) Next() <-chan struct{} {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.ch
}

// Landed records a successful commit and wakes every writer waiting on Next.
func (g *CommitGate) Landed() {
	g.mu.Lock()
	defer g.mu.Unlock()
	close(g.ch)
	g.ch = make(chan struct{})
}
