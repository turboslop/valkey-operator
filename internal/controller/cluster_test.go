package controller

import "testing"

func TestShardPrimaryNodeFallsBackToLowestOrdinalMaster(t *testing.T) {
	shard := &valkeyShard{
		id:      0,
		slotMin: 0,
		slotMax: 16383,
		nodes: []*valkeyNode{
			{name: "valkey-1", ordinal: 1, connected: true, flags: []string{"myself", "master"}},
			{name: "valkey-0", ordinal: 0, connected: true, flags: []string{"myself", "master"}},
		},
	}

	primary := shard.primaryNode()
	if primary == nil {
		t.Fatal("expected primary")
	}
	if primary.name != "valkey-0" {
		t.Fatalf("expected lowest ordinal master valkey-0, got %s", primary.name)
	}
}

func TestShardPrimaryNodePrefersCurrentSlotOwner(t *testing.T) {
	shard := &valkeyShard{
		id:      0,
		slotMin: 0,
		slotMax: 16383,
		nodes: []*valkeyNode{
			{name: "valkey-0", ordinal: 0, connected: true, flags: []string{"myself", "master"}},
			{name: "valkey-1", ordinal: 1, connected: true, flags: []string{"myself", "master"}, slots: []string{"0-16383"}},
		},
	}

	primary := shard.primaryNode()
	if primary == nil {
		t.Fatal("expected primary")
	}
	if primary.name != "valkey-1" {
		t.Fatalf("expected current slot owner valkey-1, got %s", primary.name)
	}
}
