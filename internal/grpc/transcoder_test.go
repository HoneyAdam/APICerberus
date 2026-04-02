package grpc

import (
	"testing"
)

func TestNewTranscoder(t *testing.T) {
	transcoder := NewTranscoder()
	if transcoder == nil {
		t.Fatal("NewTranscoder() returned nil")
	}
	if transcoder.files == nil {
		t.Error("files not initialized")
	}
	if transcoder.loaded {
		t.Error("loaded should be false initially")
	}
}

func TestTranscoder_IsLoaded(t *testing.T) {
	t.Run("not loaded", func(t *testing.T) {
		transcoder := NewTranscoder()
		if transcoder.IsLoaded() {
			t.Error("IsLoaded() should be false before loading descriptors")
		}
	})
}

func TestParseGRPCMethod(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		wantSvc    string
		wantMethod string
		wantErr    bool
	}{
		{
			name:       "valid method path",
			method:     "/package.Service/Method",
			wantSvc:    "package.Service",
			wantMethod: "Method",
			wantErr:    false,
		},
		{
			name:       "valid with nested package",
			method:     "/com.example.api.UserService/GetUser",
			wantSvc:    "com.example.api.UserService",
			wantMethod: "GetUser",
			wantErr:    false,
		},
		{
			name:       "without leading slash",
			method:     "package.Service/Method",
			wantSvc:    "package.Service",
			wantMethod: "Method",
			wantErr:    false,
		},
		{
			name:       "missing method name",
			method:     "/package.Service/",
			wantSvc:    "",
			wantMethod: "",
			wantErr:    true,
		},
		{
			name:       "missing service separator",
			method:     "/package.ServiceMethod",
			wantSvc:    "",
			wantMethod: "",
			wantErr:    true,
		},
		{
			name:       "empty string",
			method:     "",
			wantSvc:    "",
			wantMethod: "",
			wantErr:    true,
		},
		{
			name:       "only slash",
			method:     "/",
			wantSvc:    "",
			wantMethod: "",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, method, err := parseGRPCMethod(tt.method)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseGRPCMethod() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if svc != tt.wantSvc {
				t.Errorf("parseGRPCMethod() service = %q, want %q", svc, tt.wantSvc)
			}
			if method != tt.wantMethod {
				t.Errorf("parseGRPCMethod() method = %q, want %q", method, tt.wantMethod)
			}
		})
	}
}

func TestTranscoder_JSONToProto_NotLoaded(t *testing.T) {
	transcoder := NewTranscoder()
	_, err := transcoder.JSONToProto("/test.Service/Method", []byte(`{}`))
	if err == nil {
		t.Error("JSONToProto() should return error when not loaded")
	}
	if err != nil && err.Error() != "transcoder: proto descriptors not loaded; call LoadDescriptors before transcoding" {
		t.Logf("JSONToProto() error message: %v", err)
	}
}

func TestTranscoder_ProtoToJSON_NotLoaded(t *testing.T) {
	transcoder := NewTranscoder()
	_, err := transcoder.ProtoToJSON("/test.Service/Method", []byte{})
	if err == nil {
		t.Error("ProtoToJSON() should return error when not loaded")
	}
}

func TestTranscoder_LoadDescriptors_InvalidFile(t *testing.T) {
	transcoder := NewTranscoder()
	err := transcoder.LoadDescriptors("/nonexistent/file.desc")
	if err == nil {
		t.Error("LoadDescriptors() should return error for invalid file")
	}
}
