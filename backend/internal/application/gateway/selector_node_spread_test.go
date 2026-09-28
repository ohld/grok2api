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
	// Released at once (job-creation burst): alternate by node pick time.
	var nodes []uint64
	for range 4 {
		lease := pick()
		nodes = append(nodes, lease.Credential.EgressNodeID)
		lease.Release()
	}
	for i := 1; i < len(nodes); i++ {
		if nodes[i] == nodes[i-1] {
			t.Fatalf("burst picks did not alternate nodes: %v", nodes)
		}
	}
}
