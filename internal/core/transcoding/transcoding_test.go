package transcoding

import (
	"encoding/json"
	"testing"

	"yuelaiengine/gateway/internal/testutil"
)

func TestRouteTranscoder_JSONAndGRPCRoundTrip(t *testing.T) {
	tr := newTestRouteTranscoder(t, Options{
		DiscardUnknown: true,
	})

	request := []byte(`{"name":"alice","userId":"u-1","age":18}`)
	grpcReq, err := tr.JSONToGRPCRequest(request)
	if err != nil {
		t.Fatalf("JSONToGRPCRequest() error = %v", err)
	}

	jsonReq, err := tr.GRPCRequestToJSON(grpcReq)
	if err != nil {
		t.Fatalf("GRPCRequestToJSON() error = %v", err)
	}
	assertJSONEqual(t, jsonReq, request)

	response := []byte(`{"message":"ok","ok":true}`)
	grpcResp, err := tr.JSONToGRPCResponse(response)
	if err != nil {
		t.Fatalf("JSONToGRPCResponse() error = %v", err)
	}

	jsonResp, err := tr.GRPCResponseToJSON(grpcResp)
	if err != nil {
		t.Fatalf("GRPCResponseToJSON() error = %v", err)
	}
	assertJSONEqual(t, jsonResp, response)
}

func TestRouteTranscoder_UseProtoNamesAndEmitUnpopulated(t *testing.T) {
	tr := newTestRouteTranscoder(t, Options{
		UseProtoNames:   true,
		EmitUnpopulated: true,
		DiscardUnknown:  true,
	})

	grpcReq, err := tr.JSONToGRPCRequest([]byte(`{"name":"alice"}`))
	if err != nil {
		t.Fatalf("JSONToGRPCRequest() error = %v", err)
	}

	jsonReq, err := tr.GRPCRequestToJSON(grpcReq)
	if err != nil {
		t.Fatalf("GRPCRequestToJSON() error = %v", err)
	}
	assertJSONEqual(t, jsonReq, []byte(`{"name":"alice","user_id":"","age":0}`))
}

func TestRouteTranscoder_DiscardUnknown(t *testing.T) {
	trStrict := newTestRouteTranscoder(t, Options{
		DiscardUnknown: false,
	})
	if _, err := trStrict.JSONToGRPCRequest([]byte(`{"name":"alice","unknown":"x"}`)); err == nil {
		t.Fatalf("expected unknown field error when DiscardUnknown=false")
	}

	trLoose := newTestRouteTranscoder(t, Options{
		DiscardUnknown: true,
	})
	if _, err := trLoose.JSONToGRPCRequest([]byte(`{"name":"alice","unknown":"x"}`)); err != nil {
		t.Fatalf("unexpected error when DiscardUnknown=true: %v", err)
	}
}

func TestGRPCFrameWrapUnwrap(t *testing.T) {
	payload := []byte{0x01, 0x02, 0x03}
	frame := WrapGRPCFrame(payload)
	got, err := UnwrapGRPCFrame(frame)
	if err != nil {
		t.Fatalf("UnwrapGRPCFrame() error = %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("payload mismatch: got=%v want=%v", got, payload)
	}
}

func TestUnwrapGRPCFrameErrors(t *testing.T) {
	if _, err := UnwrapGRPCFrame([]byte{0x00, 0x01}); err == nil {
		t.Fatalf("expected short frame error")
	}
	if _, err := UnwrapGRPCFrame([]byte{0x01, 0x00, 0x00, 0x00, 0x00}); err == nil {
		t.Fatalf("expected compressed frame unsupported error")
	}
	if _, err := UnwrapGRPCFrame([]byte{0x00, 0x00, 0x00, 0x00, 0x02, 0x01}); err == nil {
		t.Fatalf("expected frame length mismatch error")
	}
}

func newTestRouteTranscoder(t *testing.T, opts Options) *RouteTranscoder {
	t.Helper()

	descPath, err := testutil.WriteDemoEchoDescriptorSet(t.TempDir())
	if err != nil {
		t.Fatalf("WriteDemoEchoDescriptorSet() error = %v", err)
	}

	opts.DescriptorPath = descPath
	opts.GRPCMethod = "/demo.v1.EchoService/Echo"

	tr, err := NewRouteTranscoder(NewDescriptorResolver(), opts)
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
