package testutil

import "testing"

func TestCommitGateWakesWaiterOnLanded(t *testing.T) {
	g := NewCommitGate()
	next := g.Next()
	select {
	case <-next:
		t.Fatal("gate fired before any commit landed")
	default:
	}
	g.Landed()
	<-next
}

func TestCommitGateCommitDuringAttemptIsNotLost(t *testing.T) {
	g := NewCommitGate()
	next := g.Next() // taken before the attempt starts
	g.Landed()       // a peer lands while the attempt is still running
	select {
	case <-next:
	default:
		t.Fatal("a commit landing mid-attempt must leave the loser's channel closed")
	}
}

func TestCommitGateResetsAfterLanded(t *testing.T) {
	g := NewCommitGate()
	g.Landed()
	next := g.Next()
	select {
	case <-next:
		t.Fatal("Next after Landed must wait for the following commit")
	default:
	}
}
