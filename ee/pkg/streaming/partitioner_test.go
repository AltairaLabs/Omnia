/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package streaming

import (
	"testing"

	"github.com/IBM/sarama"
)

func TestSessionEventPartitioner_SessionID(t *testing.T) {
	constructor := newSessionEventPartitioner(PartitionBySessionID)
	partitioner := constructor("test-topic")

	msg := &sarama.ProducerMessage{
		Key: sarama.StringEncoder("sess-abc"),
	}

	p1, err := partitioner.Partition(msg, 10)
	if err != nil {
		t.Fatalf("Partition returned error: %v", err)
	}

	// Same key should always return the same partition
	p2, err := partitioner.Partition(msg, 10)
	if err != nil {
		t.Fatalf("Partition returned error: %v", err)
	}
	if p1 != p2 {
		t.Errorf("expected same partition for same key, got %d and %d", p1, p2)
	}

	if p1 < 0 || p1 >= 10 {
		t.Errorf("partition %d out of range [0, 10)", p1)
	}
}

func TestSessionEventPartitioner_AgentID(t *testing.T) {
	constructor := newSessionEventPartitioner(PartitionByAgentID)
	partitioner := constructor("test-topic")

	msg := &sarama.ProducerMessage{
		Key: sarama.StringEncoder("agent-1"),
	}

	p1, err := partitioner.Partition(msg, 5)
	if err != nil {
		t.Fatalf("Partition returned error: %v", err)
	}
	if p1 < 0 || p1 >= 5 {
		t.Errorf("partition %d out of range [0, 5)", p1)
	}
}

func TestSessionEventPartitioner_RoundRobin(t *testing.T) {
	constructor := newSessionEventPartitioner(PartitionRoundRobin)
	partitioner := constructor("test-topic")

	msg := &sarama.ProducerMessage{}
	numPartitions := int32(3)

	seen := make(map[int32]bool)
	for i := 0; i < int(numPartitions); i++ {
		p, err := partitioner.Partition(msg, numPartitions)
		if err != nil {
			t.Fatalf("Partition returned error: %v", err)
		}
		if p < 0 || p >= numPartitions {
			t.Errorf("partition %d out of range [0, %d)", p, numPartitions)
		}
		seen[p] = true
	}

	if len(seen) != int(numPartitions) {
		t.Errorf("expected %d unique partitions, got %d", numPartitions, len(seen))
	}
}

func TestSessionEventPartitioner_ZeroPartitions(t *testing.T) {
	constructor := newSessionEventPartitioner(PartitionBySessionID)
	partitioner := constructor("test-topic")

	msg := &sarama.ProducerMessage{
		Key: sarama.StringEncoder("sess-abc"),
	}

	p, err := partitioner.Partition(msg, 0)
	if err != nil {
		t.Fatalf("Partition returned error: %v", err)
	}
	if p != 0 {
		t.Errorf("expected partition 0, got %d", p)
	}
}

func TestSessionEventPartitioner_EmptyKey(t *testing.T) {
	constructor := newSessionEventPartitioner(PartitionBySessionID)
	partitioner := constructor("test-topic")

	msg := &sarama.ProducerMessage{
		Key: sarama.StringEncoder(""),
	}

	p, err := partitioner.Partition(msg, 5)
	if err != nil {
		t.Fatalf("Partition returned error: %v", err)
	}
	if p != 0 {
		t.Errorf("expected partition 0 for empty key, got %d", p)
	}
}

func TestSessionEventPartitioner_RequiresConsistency(t *testing.T) {
	tests := []struct {
		strategy PartitionStrategy
		want     bool
	}{
		{PartitionBySessionID, true},
		{PartitionByAgentID, true},
		{PartitionRoundRobin, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.strategy), func(t *testing.T) {
			constructor := newSessionEventPartitioner(tt.strategy)
			partitioner := constructor("test-topic")
			if got := partitioner.RequiresConsistency(); got != tt.want {
				t.Errorf("RequiresConsistency() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSessionEventPartitioner_DifferentKeysDistribute(t *testing.T) {
	constructor := newSessionEventPartitioner(PartitionBySessionID)
	partitioner := constructor("test-topic")

	numPartitions := int32(100)
	partitions := make(map[int32]bool)

	// With enough different keys, we should see multiple partitions
	for i := 0; i < 50; i++ {
		msg := &sarama.ProducerMessage{
			Key: sarama.StringEncoder("sess-" + string(rune('A'+i))),
		}
		p, err := partitioner.Partition(msg, numPartitions)
		if err != nil {
			t.Fatalf("Partition returned error: %v", err)
		}
		partitions[p] = true
	}

	if len(partitions) < 2 {
		t.Error("expected keys to distribute across multiple partitions")
	}
}
