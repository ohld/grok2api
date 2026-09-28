package relational

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/chenyme/grok2api/backend/internal/domain/account"
)

func TestRoutingSkipsAccountsOnCoolingOrDisabledEgress(t *testing.T) {
	ctx := context.Background()
	database, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "egress-health.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.InitializeSchema(ctx); err != nil {
		t.Fatal(err)
	}
	accounts := NewAccountRepository(database)
	nodes := NewEgressRepository(database)
	cipher := egressOperationsCipher(t)
	healthy := createHealthyEgressNode(t, ctx, nodes, cipher, "health-ok", 0)
	cooling := createHealthyEgressNode(t, ctx, nodes, cipher, "health-cooling", 0)
	disabled := createHealthyEgressNode(t, ctx, nodes, cipher, "health-disabled", 0)
	expired := createHealthyEgressNode(t, ctx, nodes, cipher, "health-expired-cooldown", 0)
	now := time.Now().UTC()
	database.db.Model(&egressNodeModel{}).Where("id = ?", cooling.ID).Update("cooldown_until", now.Add(time.Hour))
	database.db.Model(&egressNodeModel{}).Where("id = ?", disabled.ID).Update("enabled", false)
	database.db.Model(&egressNodeModel{}).Where("id = ?", expired.ID).Update("cooldown_until", now.Add(-time.Minute))

	want := map[uint64]bool{}
	for _, node := range []struct {
		id       uint64
		routable bool
	}{{healthy.ID, true}, {cooling.ID, false}, {disabled.ID, false}, {expired.ID, true}} {
		credential := createEgressOperationsAccount(t, ctx, accounts, "health-account-"+string(rune('a'+len(want))))
		nodeID := node.id
		if _, err := accounts.UpdateEgressBindings(ctx, account.ProviderBuild, []uint64{credential.ID}, &nodeID, account.EgressAssignmentManual, now); err != nil {
			t.Fatal(err)
		}
		want[credential.ID] = node.routable
	}
	unbound := createEgressOperationsAccount(t, ctx, accounts, "health-account-unbound")
	want[unbound.ID] = true

	candidates, err := accounts.ListRoutingCandidates(ctx, account.ProviderBuild, 0, "grok-test", "")
	if err != nil {
		t.Fatal(err)
	}
	got := map[uint64]bool{}
	for _, candidate := range candidates {
		got[candidate.Credential.ID] = true
	}
	for id, routable := range want {
		if got[id] != routable {
			t.Fatalf("account %d routable = %v, want %v (candidates %v)", id, got[id], routable, got)
		}
	}
	enabled, err := accounts.ListEnabled(ctx, account.ProviderBuild)
	if err != nil || len(enabled) != len(want) {
		t.Fatalf("ListEnabled must keep every account for recovery: %d, %v", len(enabled), err)
	}
}
