package consensus

import (
	"testing"
	"time"
)

func TestNewBlockEvent(t *testing.T) {
	nodeResults := map[string]string{
		"node1": "result1",
		"node2": "result2",
	}
	consensusNodes := []string{"node1", "node2"}
	disagreeingNodes := []string{"node3"}
	metadata := map[string]string{"key": "value"}

	event := NewBlockEvent(
		"task123",
		"final_result",
		true,
		0.8,
		nodeResults,
		consensusNodes,
		disagreeingNodes,
		metadata,
	)

	if event.TaskID != "task123" {
		t.Errorf("expected TaskID task123, got %s", event.TaskID)
	}
	if event.FinalResult != "final_result" {
		t.Errorf("expected FinalResult final_result, got %s", event.FinalResult)
	}
	if !event.IsValid {
		t.Error("expected IsValid true")
	}
	if event.AgreementRate != 0.8 {
		t.Errorf("expected AgreementRate 0.8, got %f", event.AgreementRate)
	}
	if event.Timestamp == 0 {
		t.Error("expected non-zero timestamp")
	}
}

func TestNewEventChannel(t *testing.T) {
	ch := NewEventChannel(5)
	if ch == nil {
		t.Fatal("expected non-nil channel")
	}
	if cap(ch) != 5 {
		t.Errorf("expected capacity 5, got %d", cap(ch))
	}

	ch2 := NewEventChannel(0)
	if cap(ch2) != 10 {
		t.Errorf("expected default capacity 10, got %d", cap(ch2))
	}

	ch3 := NewEventChannel(-1)
	if cap(ch3) != 10 {
		t.Errorf("expected default capacity 10 for negative input, got %d", cap(ch3))
	}
}

func TestNewConsensusEvent(t *testing.T) {
	blockHash := []byte("hash123")
	data := "test data"

	event := NewConsensusEvent(EventTaskVerified, blockHash, 100, data)

	if event.Type != EventTaskVerified {
		t.Errorf("expected type EventTaskVerified, got %s", event.Type)
	}
	if string(event.BlockHash) != string(blockHash) {
		t.Errorf("expected block hash %s, got %s", string(blockHash), string(event.BlockHash))
	}
	if event.Height != 100 {
		t.Errorf("expected height 100, got %d", event.Height)
	}
	if event.Data != data {
		t.Errorf("expected data %v, got %v", data, event.Data)
	}
	if event.Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}
}

func TestEventTypeConstants(t *testing.T) {
	tests := []struct {
		name   string
		evType EventType
		value  string
	}{
		{"TaskVerified", EventTaskVerified, "task_verified"},
		{"BlockProduced", EventBlockProduced, "block_produced"},
		{"BlockVerified", EventBlockVerified, "block_verified"},
		{"ChainReorganized", EventChainReorganized, "chain_reorganized"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.evType) != tt.value {
				t.Errorf("expected %s, got %s", tt.value, tt.evType)
			}
		})
	}
}

func TestBlockEventSendReceive(t *testing.T) {
	ch := NewEventChannel(1)

	event := &BlockEvent{
		TaskID:         "test-task",
		FinalResult:    "result",
		IsValid:        true,
		AgreementRate:  1.0,
		NodeResults:    map[string]string{"n1": "r1"},
		ConsensusNodes: []string{"n1"},
		Timestamp:      time.Now().Unix(),
	}

	select {
	case ch <- event:
		// Successfully sent
	default:
		t.Fatal("failed to send event")
	}

	select {
	case received := <-ch:
		if received.TaskID != event.TaskID {
			t.Errorf("expected task ID %s, got %s", event.TaskID, received.TaskID)
		}
	default:
		t.Fatal("failed to receive event")
	}
}

func TestEventChannelTypes(t *testing.T) {
	// Test that EventChannel is correctly typed as chan *BlockEvent
	var ch EventChannel
	ch = make(chan *BlockEvent, 1)

	event := &BlockEvent{TaskID: "test"}

	go func() {
		ch <- event
	}()

	received := <-ch
	if received.TaskID != "test" {
		t.Errorf("expected task ID test, got %s", received.TaskID)
	}
}
