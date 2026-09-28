package gateway

import (
	"context"
	"testing"
	"time"

	"github.com/chenyme/grok2api/backend/internal/domain/account"
	"github.com/chenyme/grok2api/backend/internal/infra/runtime/memory"
)

func TestSelectorSpreadsWebAccountsAcrossEgressNodes(t *testing.T) {
	ctx := context.Background()
	selector := NewSelector(nil, memory.NewConcurrencyLimiter(), nil, nil, time.Hour, time.Second, time.Minute)
	values := []account.RoutingCandidate{}
	for id := uint64(1); id <= 6; id++ {
		node := uint64(23)
		if id > 4 {
			node = 32 // lower ids on node 23 used to win every tie
		}
		values = append(values, account.RoutingCandidate{Credential: account.Credential{ID: id, Priority: 1, Provider: account.ProviderWeb, EgressNodeID: node}})
	}
	pick := func() *accountLease {
		t.Helper()
		plan, err := selector.planCandidates(ctx, values, time.Now().UTC(), nil)
		if err != nil {
			t.Fatal(err)
		}
		candidate, ok := plan.Next()
		if !ok {
			t.Fatal("no candidate")
		}
		lease, err := selector.claimAccountSlot(ctx, candidate.Credential)
		if err != nil || lease == nil {
			t.Fatalf("claim %d: %v", candidate.Credential.ID, err)
		}
		return lease
	}
	// Held leases: the busier node loses.
	first, second := pick(), pick()
	if first.Credential.EgressNodeID == second.Credential.EgressNodeID {
		t.Fatalf("both held leases on node %d", first.Credential.EgressNodeID)
	}
	first.Release()
	second.Release()
	// Job-creation burst: each selection lease is released at once and the
	// job later re-acquires its account pinned and holds it while it runs.
	// Here job 3 is created before job 2's pinned claim lands.
	pinned := context.WithValue(ctx, pinnedClaimKey{}, true)
	create := func() account.Credential {
		lease := pick()
		lease.Release()
		return lease.Credential
	}
	run := func(credential account.Credential) {
		lease, err := selector.claimAccountSlot(pinned, credential)
		if err != nil || lease == nil {
			t.Fatalf("pinned claim %d: %v", credential.ID, err)
		}
		t.Cleanup(lease.Release)
	}
	job1 := create()
	run(job1)
	job2 := create()
	job3 := create()
	run(job2)
	run(job3)
	job4 := create()
	nodes := []uint64{job1.EgressNodeID, job2.EgressNodeID, job3.EgressNodeID, job4.EgressNodeID}
	for i := 1; i < len(nodes); i++ {
		if nodes[i] == nodes[i-1] {
			t.Fatalf("burst picks did not alternate nodes: %v", nodes)
		}
	}
}
