package grpc

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// Proxy handles gRPC request proxying to upstream servers.
type Proxy struct {
	// Target is the upstream gRPC server address (e.g., "localhost:50051")
	Target string

	// EnableWeb enables gRPC-Web support
	EnableWeb bool

	// Transcoding enables JSON transcoding for REST requests
	Transcoding bool

	// Transcoder handles JSON<->Proto conversion when Transcoding is enabled.
	Transcoder *Transcoder

	// StreamProxy handles gRPC streaming RPCs over HTTP.
	StreamProxy *StreamProxy

	// client is the gRPC client connection
	client *grpc.ClientConn
}

// ProxyConfig configures the gRPC proxy.
type ProxyConfig struct {
	Target            string
	EnableWeb         bool
	EnableTranscoding bool
	Insecure          bool
}

// NewProxy creates a new gRPC proxy.
func NewProxy(cfg *ProxyConfig) (*Proxy, error) {
	opts := []grpc.DialOption{
		grpc.WithDefaultCallOptions(grpc.ForceCodec(&rawCodec{})),
	}

	if cfg.Insecure {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	conn, err := grpc.Dial(cfg.Target, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to dial upstream: %w", err)
	}

	return &Proxy{
		Target:      cfg.Target,
		EnableWeb:   cfg.EnableWeb,
		Transcoding: cfg.EnableTranscoding,
		Transcoder:  NewTranscoder(),
		StreamProxy: NewStreamProxy(),
		client:      conn,
	}, nil
}

// Close closes the proxy connection.
func (p *Proxy) Close() error {
	if p.client != nil {
		return p.client.Close()
	}
	return nil
}

// ServeHTTP implements http.Handler for gRPC proxying.
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if IsGRPCWebRequest(r) && p.EnableWeb {
		p.handleGRPCWeb(w, r)
		return
	}

	if IsGRPCRequest(r) {
		p.handleGRPC(w, r)
		return
	}

	if p.Transcoding {
		p.handleTranscoding(w, r)
		return
	}

	http.Error(w, "unsupported protocol", http.StatusBadRequest)
}

// handleGRPC handles native gRPC requests.
func (p *Proxy) handleGRPC(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Extract gRPC metadata from headers
	md := metadataFromHeaders(r.Header)
	ctx = metadata.NewOutgoingContext(ctx, md)

	// Build full method path
	method := r.URL.Path
	if !strings.HasPrefix(method, "/") {
		method = "/" + method
	}

	// Read request body
	body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
	if err != nil {
		writeGRPCError(w, codes.Internal, fmt.Sprintf("failed to read body: %v", err))
		return
	}
	defer r.Body.Close()

	// Prepare response buffer
	var respBuf bytes.Buffer
	var headerMD, trailerMD metadata.MD

	// Make the gRPC call
	grpcOpts := []grpc.CallOption{
		grpc.Header(&headerMD),
		grpc.Trailer(&trailerMD),
	}

	err = p.client.Invoke(ctx, method, body, &respBuf, grpcOpts...)

	// Write response headers
	w.Header().Set("Content-Type", "application/grpc+proto")

	// Convert gRPC metadata to HTTP headers
	for k, v := range headerMD {
		for _, val := range v {
			w.Header().Add(k, val)
		}
	}

	// Set gRPC status
	grpcStatus := codes.OK
	grpcMessage := ""
	if err != nil {
		st, ok := status.FromError(err)
		if ok {
			grpcStatus = st.Code()
			grpcMessage = st.Message()
		} else {
			grpcStatus = codes.Internal
			grpcMessage = err.Error()
		}
	}

	// Write gRPC status as trailers
	w.Header().Set("Grpc-Status", fmt.Sprintf("%d", grpcStatus))
	if grpcMessage != "" {
		w.Header().Set("Grpc-Message", grpcMessage)
	}

	// Write response
	w.WriteHeader(http.StatusOK)
	w.Write(respBuf.Bytes())

	// Write trailers
	for k, v := range trailerMD {
		for _, val := range v {
			w.Header().Add(http.TrailerPrefix+k, val)
		}
	}
}

// handleGRPCWeb handles gRPC-Web requests.
func (p *Proxy) handleGRPCWeb(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Extract gRPC metadata from headers
	md := metadataFromHeaders(r.Header)
	ctx = metadata.NewOutgoingContext(ctx, md)

	// Build method path
	method := r.URL.Path
	if !strings.HasPrefix(method, "/") {
		method = "/" + method
	}

	// Read and decode body (gRPC-Web may be base64 encoded)
	body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to read body: %v", err), http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	// Check if base64 encoded (grpc-web-text)
	contentType := r.Header.Get("Content-Type")
	if strings.Contains(contentType, "text") {
		decoded, err := base64.StdEncoding.DecodeString(string(body))
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to decode base64: %v", err), http.StatusBadRequest)
			return
		}
		body = decoded
	}

	// Make gRPC call
	var respBuf bytes.Buffer
	err = p.client.Invoke(ctx, method, body, &respBuf)

	// Set response headers
	w.Header().Set("Content-Type", "application/grpc-web+proto")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Expose-Headers", "grpc-status, grpc-message")

	// Set gRPC status in headers (gRPC-Web uses headers for status)
	grpcStatus := codes.OK
	grpcMessage := "OK"
	if err != nil {
		st, ok := status.FromError(err)
		if ok {
			grpcStatus = st.Code()
			grpcMessage = st.Message()
		} else {
			grpcStatus = codes.Internal
			grpcMessage = err.Error()
		}
	}

	w.Header().Set("grpc-status", fmt.Sprintf("%d", grpcStatus))
	if grpcMessage != "" {
		w.Header().Set("grpc-message", grpcMessage)
	}

	w.WriteHeader(http.StatusOK)
	w.Write(respBuf.Bytes())
}

// handleTranscoding handles JSON to gRPC transcoding.
func (p *Proxy) handleTranscoding(w http.ResponseWriter, r *http.Request) {
	// Extract method from path
	// Expected: /v1/{service}/{method}
	method := r.URL.Path
	if !strings.HasPrefix(method, "/") {
		method = "/" + method
	}

	// Read JSON body
	jsonBody, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to read body: %v", err), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Ensure proto descriptors are loaded for proper transcoding.
	if p.Transcoder == nil || !p.Transcoder.IsLoaded() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]any{
			"code":    int(codes.FailedPrecondition),
			"message": "gRPC transcoding is not available: proto descriptors have not been loaded",
		})
		return
	}

	// Convert JSON request to protobuf.
	protoBody, err := p.Transcoder.JSONToProto(method, jsonBody)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{
			"code":    int(codes.InvalidArgument),
			"message": fmt.Sprintf("failed to transcode request: %v", err),
		})
		return
	}

	ctx := metadata.NewOutgoingContext(r.Context(), metadataFromHeaders(r.Header))

	var respBuf bytes.Buffer
	err = p.client.Invoke(ctx, method, protoBody, &respBuf)

	w.Header().Set("Content-Type", "application/json")

	if err != nil {
		st, ok := status.FromError(err)
		if ok {
			w.WriteHeader(GRPCStatusToHTTP(st.Code()))
			json.NewEncoder(w).Encode(map[string]any{
				"code":    st.Code(),
				"message": st.Message(),
				"details": st.Details(),
			})
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Convert protobuf response to JSON.
	jsonResp, err := p.Transcoder.ProtoToJSON(method, respBuf.Bytes())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{
			"code":    int(codes.Internal),
			"message": fmt.Sprintf("failed to transcode response: %v", err),
		})
		return
	}

	w.Write(jsonResp)
}

// metadataFromHeaders converts HTTP headers to gRPC metadata.
func metadataFromHeaders(headers http.Header) metadata.MD {
	md := make(metadata.MD)
	for k, v := range headers {
		// Skip HTTP-specific headers
		if isHTTPHeader(k) {
			continue
		}
		// Convert to lowercase for gRPC
		key := strings.ToLower(k)
		md[key] = v
	}
	return md
}

// isHTTPHeader returns true if the header is HTTP-specific and should not be forwarded.
func isHTTPHeader(key string) bool {
	httpHeaders := []string{
		"Accept", "Accept-Encoding", "Accept-Language",
		"Connection", "Content-Length", "Content-Type",
		"Host", "Transfer-Encoding", "User-Agent",
		"Upgrade", "Proxy-Authorization", "TE",
	}
	for _, h := range httpHeaders {
		if strings.EqualFold(key, h) {
			return true
		}
	}
	return false
}

// writeGRPCError writes a gRPC error response.
func writeGRPCError(w http.ResponseWriter, code codes.Code, message string) {
	w.Header().Set("Content-Type", "application/grpc")
	w.Header().Set("Grpc-Status", fmt.Sprintf("%d", code))
	w.Header().Set("Grpc-Message", message)
	w.WriteHeader(http.StatusOK)
}

// rawCodec is a codec that passes through raw bytes.
type rawCodec struct{}

func (c *rawCodec) Marshal(v any) ([]byte, error) {
	if b, ok := v.(*bytes.Buffer); ok {
		return b.Bytes(), nil
	}
	if b, ok := v.([]byte); ok {
		return b, nil
	}
	if b, ok := v.(*[]byte); ok {
		return *b, nil
	}
	return nil, fmt.Errorf("rawCodec: unsupported type %T", v)
}

func (c *rawCodec) Unmarshal(data []byte, v any) error {
	if b, ok := v.(*bytes.Buffer); ok {
		b.Write(data)
		return nil
	}
	if b, ok := v.(*[]byte); ok {
		*b = data
		return nil
	}
	return fmt.Errorf("rawCodec: unsupported type %T", v)
}

func (c *rawCodec) Name() string {
	return "raw"
}

// GRPCStatusToHTTP maps gRPC status codes to HTTP status codes.
func GRPCStatusToHTTP(code codes.Code) int {
	switch code {
	case codes.OK:
		return http.StatusOK
	case codes.Canceled:
		return 499 // Client Closed Request
	case codes.Unknown:
		return http.StatusInternalServerError
	case codes.InvalidArgument:
		return http.StatusBadRequest
	case codes.DeadlineExceeded:
		return http.StatusGatewayTimeout
	case codes.NotFound:
		return http.StatusNotFound
	case codes.AlreadyExists:
		return http.StatusConflict
	case codes.PermissionDenied:
		return http.StatusForbidden
	case codes.ResourceExhausted:
		return http.StatusTooManyRequests
	case codes.FailedPrecondition:
		return http.StatusPreconditionFailed
	case codes.Aborted:
		return http.StatusConflict
	case codes.OutOfRange:
		return http.StatusBadRequest
	case codes.Unimplemented:
		return http.StatusNotImplemented
	case codes.Internal:
		return http.StatusInternalServerError
	case codes.Unavailable:
		return http.StatusServiceUnavailable
	case codes.DataLoss:
		return http.StatusInternalServerError
	case codes.Unauthenticated:
		return http.StatusUnauthorized
	default:
		return http.StatusInternalServerError
	}
}

// HTTPStatusToGRPC maps HTTP status codes to gRPC status codes.
func HTTPStatusToGRPC(status int) codes.Code {
	switch status {
	case http.StatusOK:
		return codes.OK
	case http.StatusBadRequest:
		return codes.InvalidArgument
	case http.StatusUnauthorized:
		return codes.Unauthenticated
	case http.StatusForbidden:
		return codes.PermissionDenied
	case http.StatusNotFound:
		return codes.NotFound
	case http.StatusConflict:
		return codes.AlreadyExists
	case http.StatusPreconditionFailed:
		return codes.FailedPrecondition
	case http.StatusTooManyRequests:
		return codes.ResourceExhausted
	case http.StatusInternalServerError:
		return codes.Internal
	case http.StatusNotImplemented:
		return codes.Unimplemented
	case http.StatusBadGateway:
		return codes.Unavailable
	case http.StatusServiceUnavailable:
		return codes.Unavailable
	case http.StatusGatewayTimeout:
		return codes.DeadlineExceeded
	default:
		return codes.Unknown
	}
}
