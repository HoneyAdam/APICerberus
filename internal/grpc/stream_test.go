package grpc

import (
	"testing"

	"google.golang.org/grpc"
)

func TestNewStreamProxy(t *testing.T) {
	sp := NewStreamProxy()
	if sp == nil {
		t.Fatal("NewStreamProxy() returned nil")
	}
	if sp.codec == nil {
		t.Error("codec not initialized")
	}
}

func TestStreamDesc(t *testing.T) {
	tests := []struct {
		name         string
		serverStream bool
		clientStream bool
	}{
		{"server streaming", true, false},
		{"client streaming", false, true},
		{"bidirectional", true, true},
		{"unary", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desc := streamDesc(tt.serverStream, tt.clientStream)
			if desc == nil {
				t.Fatal("streamDesc() returned nil")
			}
			if desc.ServerStreams != tt.serverStream {
				t.Errorf("ServerStreams = %v, want %v", desc.ServerStreams, tt.serverStream)
			}
			if desc.ClientStreams != tt.clientStream {
				t.Errorf("ClientStreams = %v, want %v", desc.ClientStreams, tt.clientStream)
			}
		})
	}
}

func TestStreamDesc_StreamType(t *testing.T) {
	// Server streaming
	desc := streamDesc(true, false)
	if !desc.ServerStreams || desc.ClientStreams {
		t.Error("Expected server-streaming desc")
	}

	// Client streaming
	desc = streamDesc(false, true)
	if desc.ServerStreams || !desc.ClientStreams {
		t.Error("Expected client-streaming desc")
	}

	// Bidi streaming
	desc = streamDesc(true, true)
	if !desc.ServerStreams || !desc.ClientStreams {
		t.Error("Expected bidi-streaming desc")
	}

	// Unary (no streaming)
	desc = streamDesc(false, false)
	if desc.ServerStreams || desc.ClientStreams {
		t.Error("Expected unary desc")
	}
}

// Verify streamDesc returns proper grpc.StreamDesc
func TestStreamDesc_Type(t *testing.T) {
	desc := streamDesc(true, false)
	// Ensure it implements the interface
	var _ *grpc.StreamDesc = desc
}
