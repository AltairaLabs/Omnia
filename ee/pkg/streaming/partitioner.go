/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package streaming

import (
	"hash"
	"hash/fnv"

	"github.com/IBM/sarama"
)

// sessionEventPartitioner implements sarama.Partitioner using a configurable
// key extracted from the message headers or key field.
type sessionEventPartitioner struct {
	strategy PartitionStrategy
	hasher   hash.Hash32
	counter  int32
}

// newSessionEventPartitioner creates a partitioner constructor for the given strategy.
func newSessionEventPartitioner(strategy PartitionStrategy) sarama.PartitionerConstructor {
	return func(topic string) sarama.Partitioner {
		return &sessionEventPartitioner{
			strategy: strategy,
			hasher:   fnv.New32a(),
		}
	}
}

// Partition returns the partition for a given message.
func (p *sessionEventPartitioner) Partition(message *sarama.ProducerMessage, numPartitions int32) (int32, error) {
	if numPartitions <= 0 {
		return 0, nil
	}

	if p.strategy == PartitionRoundRobin {
		partition := p.counter % numPartitions
		p.counter++
		return partition, nil
	}

	// For session_id and agent_id strategies, the key is set on the message.
	keyBytes, err := message.Key.Encode()
	if err != nil || len(keyBytes) == 0 {
		return 0, nil
	}

	p.hasher.Reset()
	// Hash.Write never returns an error per the hash.Hash contract.
	_, _ = p.hasher.Write(keyBytes)

	partition := int32(p.hasher.Sum32()) % numPartitions
	if partition < 0 {
		partition = -partition
	}

	return partition, nil
}

// RequiresConsistency returns true for key-based strategies so that
// messages with the same key go to the same partition.
func (p *sessionEventPartitioner) RequiresConsistency() bool {
	return p.strategy != PartitionRoundRobin
}
