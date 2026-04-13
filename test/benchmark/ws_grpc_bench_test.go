package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ============================================================================
// WebSocket Throughput Benchmarks
// ============================================================================

// BenchmarkWebSocketMessageThroughput benchmarks WebSocket message encoding/
// decoding throughput. Full WebSocket benchmarks require live network
// connections; this measures the message framing layer.
func BenchmarkWebSocketMessageThroughput(b *testing.B) {
	// Simulate WebSocket message payloads of different sizes
	payloads := map[string][]byte{
		"small_64B":  make([]byte, 64),
		"medium_1KB": make([]byte, 1024),
		"large_64KB": make([]byte, 64*1024),
	}
	for name, payload := range payloads {
		for i := range payload {
			payload[i] = byte('A' + (i % 26))
		}
		b.Run(name, func(b *testing.B) {
			b.SetBytes(int64(len(payload)))
			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				// Simulate WebSocket frame: opcode(1) + mask(1) + length + payload + mask-key(4)
				frame := buildWSFrame(0x81, payload)
				_ = frame
			}
		})
	}
}

// buildWSFrame builds a minimal WebSocket frame (RFC 6455).
func buildWSFrame(opcode byte, payload []byte) []byte {
	var buf bytes.Buffer
	buf.WriteByte(opcode)

	length := len(payload)
	switch {
	case length <= 125:
		buf.WriteByte(byte(length))
	case length <= 65535:
		buf.WriteByte(126)
		buf.WriteByte(byte(length >> 8))
		buf.WriteByte(byte(length))
	default:
		buf.WriteByte(127)
		for i := 7; i >= 0; i-- {
			buf.WriteByte(byte(length >> (i * 8)))
		}
	}

	buf.Write(payload)
	return buf.Bytes()
}

// BenchmarkWebSocketConcurrentMessages benchmarks concurrent WebSocket
// message processing with multiple clients sending messages simultaneously.
func BenchmarkWebSocketConcurrentMessages(b *testing.B) {
	clientCounts := []int{1, 10, 100}
	for _, clients := range clientCounts {
		b.Run(fmt.Sprintf("clients=%d", clients), func(b *testing.B) {
			var totalMsgs atomic.Int64
			msgSize := 256 // 256-byte messages
			payload := make([]byte, msgSize)
			for i := range payload {
				payload[i] = byte('a' + (i % 26))
			}

			var wg sync.WaitGroup
			msgsPerClient := b.N / clients
			if msgsPerClient == 0 {
				msgsPerClient = 1
			}

			b.ResetTimer()
			for c := 0; c < clients; c++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for i := 0; i < msgsPerClient; i++ {
						frame := buildWSFrame(0x81, payload)
						// Simulate decode: read frame length
						if len(frame) > 2 {
							totalMsgs.Add(1)
						}
					}
				}()
			}
			wg.Wait()
			b.ReportMetric(float64(totalMsgs.Load()), "msgs_total")
		})
	}
}

// BenchmarkWebSocketPingPongLatency measures round-trip latency for
// ping/pong control frames (minimal frame build + parse).
func BenchmarkWebSocketPingPongLatency(b *testing.B) {
	pingPayload := []byte("ping-12345")

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		// Build ping frame
		pingFrame := buildWSFrame(0x89, pingPayload)
		// Build pong frame (same payload, different opcode)
		pongFrame := buildWSFrame(0x8A, pingPayload)
		// Verify payload matches (simulates pong validation)
		if !bytes.Equal(pingFrame[2:], pongFrame[2:]) {
			b.Fatal("pong payload mismatch")
		}
	}
}

// ============================================================================
// gRPC Transcoding Overhead Benchmarks
// ============================================================================

// BenchmarkGRPCTranscodingJSONToProto simulates JSON-to-protobuf conversion
// overhead by measuring JSON marshal/unmarshal of typical gRPC message payloads.
func BenchmarkGRPCTranscodingJSONToProto(b *testing.B) {
	// Simulated gRPC message as JSON (what transcoding receives)
	type UserMessage struct {
		ID    string   `json:"id"`
		Name  string   `json:"name"`
		Email string   `json:"email"`
		Tags  []string `json:"tags"`
	}

	type ListResponse struct {
		Users      []UserMessage `json:"users"`
		TotalCount int           `json:"total_count"`
		Page       int           `json:"page"`
	}

	resp := ListResponse{
		TotalCount: 1000,
		Page:       1,
		Users:      make([]UserMessage, 10),
	}
	for i := range resp.Users {
		resp.Users[i] = UserMessage{
			ID:    fmt.Sprintf("user-%d", i),
			Name:  fmt.Sprintf("User %d", i),
			Email: fmt.Sprintf("user%d@example.com", i),
			Tags:  []string{"admin", "active"},
		}
	}

	b.Run("json_marshal", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			data, err := json.Marshal(resp)
			if err != nil {
				b.Fatal(err)
			}
			_ = data
		}
	})

	b.Run("json_unmarshal", func(b *testing.B) {
		data, _ := json.Marshal(resp)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var out ListResponse
			if err := json.Unmarshal(data, &out); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("round_trip", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			data, err := json.Marshal(resp)
			if err != nil {
				b.Fatal(err)
			}
			var out ListResponse
			if err := json.Unmarshal(data, &out); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkGRPCTranscodingOverhead measures the overhead of HTTP transcoding
// by comparing raw HTTP response vs transcoded response times.
func BenchmarkGRPCTranscodingOverhead(b *testing.B) {
	// Upstream that returns a "protobuf-like" binary response
	protoPayload := make([]byte, 512)
	for i := range protoPayload {
		protoPayload[i] = byte(i % 256)
	}

	// Simulate transcoding: binary → JSON
	transcodeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate "transcoding" by parsing binary and returning JSON
		type Response struct {
			Data   string `json:"data"`
			Length int    `json:"length"`
		}
		resp := Response{
			Data:   fmt.Sprintf("transcoded-%d-bytes", len(protoPayload)),
			Length: len(protoPayload),
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer transcodeServer.Close()

	// Raw server that returns binary directly
	rawServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(protoPayload)
	}))
	defer rawServer.Close()

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
		},
	}

	b.Run("raw_binary", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			resp, err := client.Get(rawServer.URL + "/data")
			if err != nil {
				b.Fatal(err)
			}
			resp.Body.Close()
		}
	})

	b.Run("transcoded_json", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			resp, err := client.Get(transcodeServer.URL + "/data")
			if err != nil {
				b.Fatal(err)
			}
			resp.Body.Close()
		}
	})
}

// BenchmarkGRPCTranscodingLargePayload measures transcoding overhead with
// large message bodies (simulating ListUsers with many results).
func BenchmarkGRPCTranscodingLargePayload(b *testing.B) {
	type Item struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
	}

	sizes := map[string]int{
		"10_items":   10,
		"100_items":  100,
		"1000_items": 1000,
	}

	for name, count := range sizes {
		b.Run(name, func(b *testing.B) {
			items := make([]Item, count)
			for i := range items {
				items[i] = Item{
					ID:          fmt.Sprintf("item-%d", i),
					Name:        fmt.Sprintf("Item %d with a moderately long name", i),
					Description: fmt.Sprintf("Description for item %d with some additional text to make it more realistic", i),
				}
			}

			b.Run("marshal", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					data, err := json.Marshal(items)
					if err != nil {
						b.Fatal(err)
					}
					_ = data
				}
			})

			b.Run("unmarshal", func(b *testing.B) {
				data, _ := json.Marshal(items)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					var out []Item
					if err := json.Unmarshal(data, &out); err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}
