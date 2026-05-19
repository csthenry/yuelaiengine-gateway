package proxy

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"yuelaiengine/gateway/internal/core/transcoding"
	"yuelaiengine/gateway/internal/testutil"
)

func TestApplyRequestTranscoding_HTTPJSONToGRPC(t *testing.T) {
	tr := newProxyTestTranscoder(t)
	req := httptest.NewRequest(http.MethodPost, "http://example.com/echo", bytes.NewReader([]byte(`{"name":"alice","userId":"u-1","age":18}`)))
	req.Header.Set("Content-Type", "application/json")

	if err := applyRequestTranscoding(req, "http_json_to_grpc", tr); err != nil {
		t.Fatalf("applyRequestTranscoding() error = %v", err)
	}
	if got := req.Header.Get("Content-Type"); got != "application/grpc" {
		t.Fatalf("content-type mismatch: got=%s want=application/grpc", got)
	}

	raw, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("read request body failed: %v", err)
	}
	jsonBody, err := tr.GRPCRequestToJSON(raw)
	if err != nil {
		t.Fatalf("GRPCRequestToJSON() error = %v", err)
	}
	assertJSONEqual(t, jsonBody, []byte(`{"name":"alice","userId":"u-1","age":18}`))
}

func TestApplyRequestTranscoding_GRPCToHTTPJSON(t *testing.T) {
	tr := newProxyTestTranscoder(t)
	grpcBody, err := tr.JSONToGRPCRequest([]byte(`{"name":"alice","userId":"u-1","age":18}`))
	if err != nil {
		t.Fatalf("JSONToGRPCRequest() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "http://example.com/echo", bytes.NewReader(grpcBody))
	req.Header.Set("Content-Type", "application/grpc")
	if err := applyRequestTranscoding(req, "grpc_to_http_json", tr); err != nil {
		t.Fatalf("applyRequestTranscoding() error = %v", err)
	}
	if got := req.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("content-type mismatch: got=%s want=application/json", got)
	}

	jsonBody, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("read request body failed: %v", err)
	}
	assertJSONEqual(t, jsonBody, []byte(`{"name":"alice","userId":"u-1","age":18}`))
}

func TestApplyGRPCToJSONResponseTranscoding_Success(t *testing.T) {
	tr := newProxyTestTranscoder(t)
	grpcBody, err := tr.JSONToGRPCResponse([]byte(`{"message":"ok","ok":true}`))
	if err != nil {
		t.Fatalf("JSONToGRPCResponse() error = %v", err)
	}

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"application/grpc"},
			"Grpc-Status":  []string{"0"},
		},
		Body: io.NopCloser(bytes.NewReader(grpcBody)),
	}

	if err := applyGRPCToJSONResponseTranscoding(resp, tr); err != nil {
		t.Fatalf("applyGRPCToJSONResponseTranscoding() error = %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status mismatch: got=%d want=%d", resp.StatusCode, http.StatusOK)
	}
	if got := resp.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("content-type mismatch: got=%s want=application/json", got)
	}
	if got := resp.Header.Get("Grpc-Status"); got != "" {
		t.Fatalf("expected grpc headers cleaned, got Grpc-Status=%q", got)
	}

	jsonBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body failed: %v", err)
	}
	assertJSONEqual(t, jsonBody, []byte(`{"message":"ok","ok":true}`))
}

func TestApplyGRPCToJSONResponseTranscoding_GRPCError(t *testing.T) {
	tr := newProxyTestTranscoder(t)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"application/grpc"},
			"Grpc-Status":  []string{"5"},
			"Grpc-Message": []string{"not%20found"},
		},
		Body: io.NopCloser(bytes.NewReader(transcoding.WrapGRPCFrame(nil))),
	}

	if err := applyGRPCToJSONResponseTranscoding(resp, tr); err != nil {
		t.Fatalf("applyGRPCToJSONResponseTranscoding() error = %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status mismatch: got=%d want=%d", resp.StatusCode, http.StatusNotFound)
	}
	if got := resp.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("content-type mismatch: got=%s want=application/json", got)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body failed: %v", err)
	}
	assertJSONEqual(t, body, []byte(`{"error":{"grpc_status":"5","grpc_message":"not found"}}`))
}

func TestApplyJSONToGRPCResponseTranscoding_Success(t *testing.T) {
	tr := newProxyTestTranscoder(t)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader([]byte(`{"message":"ok","ok":true}`))),
	}

	if err := applyJSONToGRPCResponseTranscoding(resp, tr); err != nil {
		t.Fatalf("applyJSONToGRPCResponseTranscoding() error = %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status mismatch: got=%d want=%d", resp.StatusCode, http.StatusOK)
	}
	if got := resp.Header.Get("Content-Type"); got != "application/grpc" {
		t.Fatalf("content-type mismatch: got=%s want=application/grpc", got)
	}
	if got := resp.Header.Get("Grpc-Status"); got != "0" {
		t.Fatalf("grpc-status mismatch: got=%s want=0", got)
	}

	grpcBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body failed: %v", err)
	}
	jsonBody, err := tr.GRPCResponseToJSON(grpcBody)
	if err != nil {
		t.Fatalf("GRPCResponseToJSON() error = %v", err)
	}
	assertJSONEqual(t, jsonBody, []byte(`{"message":"ok","ok":true}`))
}

func TestApplyJSONToGRPCResponseTranscoding_HTTPErrorToGRPCError(t *testing.T) {
	tr := newProxyTestTranscoder(t)
	resp := &http.Response{
		StatusCode: http.StatusNotFound,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader([]byte("missing data"))),
	}

	if err := applyJSONToGRPCResponseTranscoding(resp, tr); err != nil {
		t.Fatalf("applyJSONToGRPCResponseTranscoding() error = %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status mismatch: got=%d want=%d", resp.StatusCode, http.StatusOK)
	}
	if got := resp.Header.Get("Content-Type"); got != "application/grpc" {
		t.Fatalf("content-type mismatch: got=%s want=application/grpc", got)
	}
	if got := resp.Header.Get("Grpc-Status"); got != "5" {
		t.Fatalf("grpc-status mismatch: got=%s want=5", got)
	}
	if got := resp.Header.Get("Grpc-Message"); got != "missing+data" {
		t.Fatalf("grpc-message mismatch: got=%q want=%q", got, "missing+data")
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body failed: %v", err)
	}
	if len(raw) != 0 {
		t.Fatalf("expected empty grpc error body, got len=%d", len(raw))
	}
}

func newProxyTestTranscoder(t *testing.T) *transcoding.RouteTranscoder {
	t.Helper()

	descPath, err := testutil.WriteDemoEchoDescriptorSet(t.TempDir())
	if err != nil {
		t.Fatalf("WriteDemoEchoDescriptorSet() error = %v", err)
	}

	tr, err := transcoding.NewRouteTranscoder(transcoding.NewDescriptorResolver(), transcoding.Options{
		DescriptorPath: descPath,
		GRPCMethod:     "/demo.v1.EchoService/Echo",
		DiscardUnknown: true,
	})
	if err != nil {
		t.Fatalf("NewRouteTranscoder() error = %v", err)
	}
	return tr
}

func assertJSONEqual(t *testing.T, got, want []byte) {
	t.Helper()

	var gotObj interface{}
	if err := json.Unmarshal(got, &gotObj); err != nil {
		t.Fatalf("unmarshal got json failed: %v, body=%s", err, string(got))
	}

	var wantObj interface{}
	if err := json.Unmarshal(want, &wantObj); err != nil {
		t.Fatalf("unmarshal want json failed: %v, body=%s", err, string(want))
	}

	gotCanonical, _ := json.Marshal(gotObj)
	wantCanonical, _ := json.Marshal(wantObj)
	if string(gotCanonical) != string(wantCanonical) {
		t.Fatalf("json mismatch: got=%s want=%s", string(gotCanonical), string(wantCanonical))
	}
}
