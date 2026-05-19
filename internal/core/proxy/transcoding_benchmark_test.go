package proxy

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"yuelaiengine/gateway/internal/core/transcoding"
	"yuelaiengine/gateway/internal/testutil"
)

func BenchmarkApplyRequestTranscoding_HTTPJSONToGRPC(b *testing.B) {
	tr := newProxyBenchTranscoder(b)
	payload := []byte(`{"name":"alice","userId":"u-1","age":18}`)

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "http://example.com/echo", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		if err := applyRequestTranscoding(req, "http_json_to_grpc", tr); err != nil {
			b.Fatalf("applyRequestTranscoding() error = %v", err)
		}
		_, _ = io.Copy(io.Discard, req.Body)
		_ = req.Body.Close()
	}
}

func BenchmarkApplyGRPCToJSONResponseTranscoding(b *testing.B) {
	tr := newProxyBenchTranscoder(b)
	grpcBody, err := tr.JSONToGRPCResponse([]byte(`{"message":"ok","ok":true}`))
	if err != nil {
		b.Fatalf("JSONToGRPCResponse() error = %v", err)
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type": []string{"application/grpc"},
				"Grpc-Status":  []string{"0"},
			},
			Body: io.NopCloser(bytes.NewReader(grpcBody)),
		}
		if err := applyGRPCToJSONResponseTranscoding(resp, tr); err != nil {
			b.Fatalf("applyGRPCToJSONResponseTranscoding() error = %v", err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
}

func newProxyBenchTranscoder(b *testing.B) *transcoding.RouteTranscoder {
	b.Helper()

	descPath, err := testutil.WriteDemoEchoDescriptorSet(b.TempDir())
	if err != nil {
		b.Fatalf("WriteDemoEchoDescriptorSet() error = %v", err)
	}

	tr, err := transcoding.NewRouteTranscoder(transcoding.NewDescriptorResolver(), transcoding.Options{
		DescriptorPath: descPath,
		GRPCMethod:     "/demo.v1.EchoService/Echo",
		DiscardUnknown: true,
	})
	if err != nil {
		b.Fatalf("NewRouteTranscoder() error = %v", err)
	}
	return tr
}
