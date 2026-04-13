package grpc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// StreamProxy handles proxying of gRPC streaming RPCs over HTTP.
type StreamProxy struct {
	// Codec used for raw byte pass-through.
	codec *rawCodec
}

// NewStreamProxy creates a new StreamProxy.
func NewStreamProxy() *StreamProxy {
	return &StreamProxy{
		codec: &rawCodec{},
	}
}

// streamDesc returns a grpc.StreamDesc configured for the given streaming mode.
func streamDesc(serverStream, clientStream bool) *grpc.StreamDesc {
	return &grpc.StreamDesc{
		ServerStreams: serverStream,
		ClientStreams: clientStream,
	}
}

// ProxyServerStream proxies a server-streaming gRPC call.
// The server sends multiple messages in response to a single client request.
// Responses are flushed to the client as newline-delimited JSON objects.
func (sp *StreamProxy) ProxyServerStream(w http.ResponseWriter, r *http.Request, conn *grpc.ClientConn, method string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported by server", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()
	md := metadataFromHeaders(r.Header)
	ctx = metadata.NewOutgoingContext(ctx, md)

	// Read the single client request body.
	body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to read request body: %v", err), http.StatusBadRequest)
		return
	}
	defer func() { _ = r.Body.Close() }() // #nosec G104

	// Open the server-streaming RPC.
	desc := streamDesc(true, false)
	stream, err := conn.NewStream(ctx, desc, method)
	if err != nil {
		writeStreamError(w, err)
		return
	}

	// Send the single client message.
	if err := stream.SendMsg(&body); err != nil {
		writeStreamError(w, err)
		_ = stream.CloseSend()
		return
	}

	// Close the send direction so the server knows we are done.
	if err := stream.CloseSend(); err != nil {
		writeStreamError(w, err)
		return
	}

	// Stream responses back.
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.WriteHeader(http.StatusOK)

	for {
		var respBuf bytes.Buffer
		if err := stream.RecvMsg(&respBuf); err != nil {
			if err == io.EOF {
				// Normal end of stream.
				break
			}
			// Write a terminal error frame so the client knows what happened.
			writeStreamErrorFrame(w, err)
			flusher.Flush()
			return
		}
		// Write the response frame followed by a newline.
		_, _ = w.Write(respBuf.Bytes()) // #nosec G104
		_, _ = w.Write([]byte("\n"))    // #nosec G104
		flusher.Flush()
	}

	// Write a final status frame.
	writeStreamStatusFrame(w, codes.OK, "")
	flusher.Flush()
}

// ProxyClientStream proxies a client-streaming gRPC call.
// The client sends multiple messages and receives a single response.
// The request body is expected to contain newline-delimited messages.
func (sp *StreamProxy) ProxyClientStream(w http.ResponseWriter, r *http.Request, conn *grpc.ClientConn, method string) {
	ctx := r.Context()
	md := metadataFromHeaders(r.Header)
	ctx = metadata.NewOutgoingContext(ctx, md)

	// Open the client-streaming RPC.
	desc := streamDesc(false, true)
	stream, err := conn.NewStream(ctx, desc, method)
	if err != nil {
		writeStreamError(w, err)
		return
	}

	// Read the request body as newline-delimited messages.
	body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to read request body: %v", err), http.StatusBadRequest)
		return
	}
	defer func() { _ = r.Body.Close() }() // #nosec G104

	messages := splitMessages(body)
	for _, msg := range messages {
		msgCopy := make([]byte, len(msg))
		copy(msgCopy, msg)
		if err := stream.SendMsg(&msgCopy); err != nil {
			writeStreamError(w, err)
			_ = stream.CloseSend()
			return
		}
	}

	// Close the send direction.
	if err := stream.CloseSend(); err != nil {
		writeStreamError(w, err)
		return
	}

	// Receive the single response.
	var respBuf bytes.Buffer
	if err := stream.RecvMsg(&respBuf); err != nil {
		writeStreamError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(respBuf.Bytes()) // #nosec G104
}

// ProxyBidiStream proxies a bidirectional streaming gRPC call.
// Both client and server send streams of messages concurrently.
// Uses WebSocket-style communication over HTTP with newline-delimited messages.
func (sp *StreamProxy) ProxyBidiStream(w http.ResponseWriter, r *http.Request, conn *grpc.ClientConn, method string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported by server", http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	md := metadataFromHeaders(r.Header)
	ctx = metadata.NewOutgoingContext(ctx, md)

	// Open the bidirectional streaming RPC.
	desc := streamDesc(true, true)
	stream, err := conn.NewStream(ctx, desc, method)
	if err != nil {
		writeStreamError(w, err)
		return
	}

	// Set headers for streaming response.
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.WriteHeader(http.StatusOK)

	// Send goroutine: reads messages from the HTTP request body and sends them upstream.
	sendDone := make(chan error, 1)
	go func() {
		defer func() {
			_ = stream.CloseSend() // #nosec G104
		}()

		body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
		if err != nil {
			sendDone <- fmt.Errorf("failed to read request body: %w", err)
			return
		}
		defer func() { _ = r.Body.Close() }() // #nosec G104

		messages := splitMessages(body)
		for _, msg := range messages {
			msgCopy := make([]byte, len(msg))
			copy(msgCopy, msg)
			if err := stream.SendMsg(&msgCopy); err != nil {
				sendDone <- err
				return
			}
		}
		sendDone <- nil
	}()

	// Receive loop: reads messages from the upstream gRPC stream and writes them to the HTTP response.
	for {
		var respBuf bytes.Buffer
		if err := stream.RecvMsg(&respBuf); err != nil {
			if err == io.EOF {
				break
			}
			writeStreamErrorFrame(w, err)
			flusher.Flush()
			return
		}
		_, _ = w.Write(respBuf.Bytes()) // #nosec G104
		_, _ = w.Write([]byte("\n"))    // #nosec G104
		flusher.Flush()
	}

	// Wait for the send goroutine to finish.
	<-sendDone

	writeStreamStatusFrame(w, codes.OK, "")
	flusher.Flush()
}

// splitMessages splits a byte slice by newlines, discarding empty segments.
func splitMessages(data []byte) [][]byte {
	var messages [][]byte
	for _, line := range bytes.Split(data, []byte("\n")) {
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) > 0 {
			messages = append(messages, trimmed)
		}
	}
	return messages
}

// writeStreamError writes a gRPC error as an HTTP error response.
// This is used before the response headers have been sent.
func writeStreamError(w http.ResponseWriter, err error) {
	st, ok := status.FromError(err)
	if ok {
		httpCode := GRPCStatusToHTTP(st.Code())
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(httpCode)
		_ = json.NewEncoder(w).Encode(map[string]any{ // #nosec G104
			"code":    int(st.Code()),
			"message": st.Message(),
		})
		return
	}
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

// writeStreamErrorFrame writes a JSON error frame into an already-started streaming response.
func writeStreamErrorFrame(w http.ResponseWriter, err error) {
	st, ok := status.FromError(err)
	code := codes.Internal
	message := err.Error()
	if ok {
		code = st.Code()
		message = st.Message()
	}

	frame, _ := json.Marshal(map[string]any{ // #nosec G104
		"error":   true,
		"code":    int(code),
		"message": message,
	})
	_, _ = w.Write(frame)        // #nosec G104
	_, _ = w.Write([]byte("\n")) // #nosec G104
}

// writeStreamStatusFrame writes a terminal status frame to signal end-of-stream.
func writeStreamStatusFrame(w http.ResponseWriter, code codes.Code, message string) {
	frame, _ := json.Marshal(map[string]any{ // #nosec G104
		"status":  int(code),
		"message": message,
	})
	_, _ = w.Write(frame)        // #nosec G104
	_, _ = w.Write([]byte("\n")) // #nosec G104
}
