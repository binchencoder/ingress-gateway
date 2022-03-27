package integration_test

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"

	examplepb "github.com/binchencoder/ease-gateway/examples/internal/proto/examplepb"
	"github.com/binchencoder/ease-gateway/gateway/runtime"
	"github.com/google/go-cmp/cmp"
	fieldmaskpb "google.golang.org/genproto/protobuf/field_mask"

	// "google.golang.org/grpc/codes"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/structpb"
)

var marshaler = &runtime.JSONPb{}

func TestEcho(t *testing.T) {
	if testing.Short() {
		t.Skip()
		return
	}

	for _, apiPrefix := range []string{"v1", "v2"} {
		t.Run(apiPrefix, func(t *testing.T) {
			testEcho(t, 8088, apiPrefix, "application/json")
			testEchoOneof(t, 8088, apiPrefix, "application/json")
			testEchoOneof1(t, 8088, apiPrefix, "application/json")
			testEchoOneof2(t, 8088, apiPrefix, "application/json")
			testEchoBody(t, 8088, apiPrefix, true)
			testEchoBody(t, 8088, apiPrefix, false)
			// Use SendHeader/SetTrailer without gRPC server https://github.com/grpc-ecosystem/grpc-gateway/issues/517#issuecomment-684625645
			testEchoBody(t, 8089, apiPrefix, true)
			testEchoBody(t, 8089, apiPrefix, false)
		})
	}
	t.Run("testEchoValidationRules", func(t *testing.T) {
		testEchoValidationRules(t, 8088, "application/json")
	})
}

func TestEchoPatch(t *testing.T) {
	if testing.Short() {
		t.Skip()
		return
	}

	sent := examplepb.DynamicMessage{
		StructField: &structpb.Struct{Fields: map[string]*structpb.Value{
			"struct_key": {Kind: &structpb.Value_StructValue{
				StructValue: &structpb.Struct{Fields: map[string]*structpb.Value{
					"layered_struct_key": {Kind: &structpb.Value_StringValue{StringValue: "struct_val"}},
				}},
			}},
		}},
		ValueField: &structpb.Value{Kind: &structpb.Value_StructValue{
			StructValue: &structpb.Struct{Fields: map[string]*structpb.Value{
				"value_struct_key": {Kind: &structpb.Value_StringValue{StringValue: "value_struct_val"}},
			}},
		}},
	}
	payload, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(&sent)
	if err != nil {
		t.Fatalf("marshaler.Marshal(%#v) failed with %v; want success", payload, err)
	}

	apiURL := "http://localhost:8088/v1/example/echo_patch"
	req, err := http.NewRequest("PATCH", apiURL, bytes.NewReader(payload))
	if err != nil {
		t.Errorf("http.NewRequest(PATCH, %q) failed with %v; want success", apiURL, err)
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Errorf("http.Post(%#v) failed with %v; want success", req, err)
		return
	}
	defer resp.Body.Close()
	buf, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Errorf("ioutil.ReadAll(resp.Body) failed with %v; want success", err)
		return
	}

	if got, want := resp.StatusCode, http.StatusOK; got != want {
		t.Errorf("resp.StatusCode = %d; want %d", got, want)
		t.Logf("%s", buf)
	}

	var received examplepb.DynamicMessageUpdate
	if err := marshaler.Unmarshal(buf, &received); err != nil {
		t.Errorf("marshaler.Unmarshal(%s, msg) failed with %v; want success", buf, err)
		return
	}
	if diff := cmp.Diff(received.Body, sent, protocmp.Transform()); diff != "" {
		t.Errorf(diff)
	}
	if diff := cmp.Diff(received.UpdateMask, fieldmaskpb.FieldMask{Paths: []string{
		"struct_field.struct_key.layered_struct_key", "value_field.value_struct_key",
	}}, protocmp.Transform(), protocmp.SortRepeatedFields(received.UpdateMask, "paths")); diff != "" {
		t.Errorf(diff)
	}
}

func TestForwardResponseOption(t *testing.T) {
	if testing.Short() {
		t.Skip()
		return
	}

	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	port := 7079
	go func() {
		if err := runGateway(
			ctx,
			fmt.Sprintf(":%d", port),
			runtime.WithForwardResponseOption(
				func(_ context.Context, w http.ResponseWriter, _ proto.Message) error {
					w.Header().Set("Content-Type", "application/vnd.docker.plugins.v1.1+json")
					return nil
				},
			),
		); err != nil {
			t.Errorf("runGateway() failed with %v; want success", err)
			return
		}
	}()
	if err := waitForGateway(ctx, uint16(port)); err != nil {
		t.Errorf("waitForGateway(ctx, %d) failed with %v; want success", port, err)
	}
	testEcho(t, port, "v1", "application/vnd.docker.plugins.v1.1+json")
}

func TestForwardResponseOptionHTTPPathPattern(t *testing.T) {
	if testing.Short() {
		t.Skip()
		return
	}

	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	port := 7080
	go func() {
		if err := runGateway(
			ctx,
			fmt.Sprintf(":%d", port),
			runtime.WithForwardResponseOption(
				func(ctx context.Context, w http.ResponseWriter, _ proto.Message) error {
					path, _ := runtime.HTTPPathPattern(ctx)
					w.Header().Set("Content-Type", path)
					return nil
				},
			),
		); err != nil {
			t.Errorf("runGateway() failed with %v; want success", err)
			return
		}
	}()
	if err := waitForGateway(ctx, uint16(port)); err != nil {
		t.Errorf("waitForGateway(ctx, %d) failed with %v; want success", port, err)
	}
	testEcho(t, port, "v1", "/v1/example/echo/{id}")
}

func testEcho(t *testing.T, port int, apiPrefix string, contentType string) {
	apiURL := fmt.Sprintf("http://localhost:%d/%s/example/echo/myid", port, apiPrefix)
	resp, err := http.Post(apiURL, "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Errorf("http.Post(%q) failed with %v; want success", apiURL, err)
		return
	}
	defer resp.Body.Close()
	buf, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Errorf("ioutil.ReadAll(resp.Body) failed with %v; want success", err)
		return
	}

	if got, want := resp.StatusCode, http.StatusOK; got != want {
		t.Errorf("resp.StatusCode = %d; want %d", got, want)
		t.Logf("%s", buf)
	}

	msg := new(examplepb.UnannotatedSimpleMessage)
	if err := marshaler.Unmarshal(buf, msg); err != nil {
		t.Errorf("marshaler.Unmarshal(%s, msg) failed with %v; want success", buf, err)
		return
	}
	if got, want := msg.Id, "myid"; got != want {
		t.Errorf("msg.Id = %q; want %q", got, want)
	}

	if value := resp.Header.Get("Content-Type"); value != contentType {
		t.Errorf("Content-Type was %s, wanted %s", value, contentType)
	}
}

func testEchoOneof(t *testing.T, port int, apiPrefix string, contentType string) {
	apiURL := fmt.Sprintf("http://localhost:%d/%s/example/echo/myid/10/golang", port, apiPrefix)
	resp, err := http.Get(apiURL)
	if err != nil {
		t.Errorf("http.Get(%q) failed with %v; want success", apiURL, err)
		return
	}
	defer resp.Body.Close()
	buf, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Errorf("ioutil.ReadAll(resp.Body) failed with %v; want success", err)
		return
	}

	if got, want := resp.StatusCode, http.StatusOK; got != want {
		t.Errorf("resp.StatusCode = %d; want %d", got, want)
		t.Logf("%s", buf)
	}

	msg := new(examplepb.UnannotatedSimpleMessage)
	if err := marshaler.Unmarshal(buf, msg); err != nil {
		t.Errorf("marshaler.Unmarshal(%s, msg) failed with %v; want success", buf, err)
		return
	}
	if got, want := msg.GetLang(), "golang"; got != want {
		t.Errorf("msg.GetLang() = %q; want %q", got, want)
	}

	if value := resp.Header.Get("Content-Type"); value != contentType {
		t.Errorf("Content-Type was %s, wanted %s", value, contentType)
	}
}

func testEchoOneof1(t *testing.T, port int, apiPrefix string, contentType string) {
	apiURL := fmt.Sprintf("http://localhost:%d/%s/example/echo1/myid/10/golang", port, apiPrefix)
	resp, err := http.Get(apiURL)
	if err != nil {
		t.Errorf("http.Get(%q) failed with %v; want success", apiURL, err)
		return
	}
	defer resp.Body.Close()
	buf, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Errorf("ioutil.ReadAll(resp.Body) failed with %v; want success", err)
		return
	}

	if got, want := resp.StatusCode, http.StatusOK; got != want {
		t.Errorf("resp.StatusCode = %d; want %d", got, want)
		t.Logf("%s", buf)
	}

	msg := new(examplepb.UnannotatedSimpleMessage)
	if err := marshaler.Unmarshal(buf, msg); err != nil {
		t.Errorf("marshaler.Unmarshal(%s, msg) failed with %v; want success", buf, err)
		return
	}
	if got, want := msg.GetStatus().GetNote(), "golang"; got != want {
		t.Errorf("msg.GetStatus().GetNote() = %q; want %q", got, want)
	}

	if value := resp.Header.Get("Content-Type"); value != contentType {
		t.Errorf("Content-Type was %s, wanted %s", value, contentType)
	}
}

func testEchoOneof2(t *testing.T, port int, apiPrefix string, contentType string) {
	apiURL := fmt.Sprintf("http://localhost:%d/%s/example/echo2/golang", port, apiPrefix)
	resp, err := http.Get(apiURL)
	if err != nil {
		t.Errorf("http.Get(%q) failed with %v; want success", apiURL, err)
		return
	}
	defer resp.Body.Close()
	buf, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Errorf("ioutil.ReadAll(resp.Body) failed with %v; want success", err)
		return
	}

	if got, want := resp.StatusCode, http.StatusOK; got != want {
		t.Errorf("resp.StatusCode = %d; want %d", got, want)
		t.Logf("%s", buf)
	}

	msg := new(examplepb.UnannotatedSimpleMessage)
	if err := marshaler.Unmarshal(buf, msg); err != nil {
		t.Errorf("marshaler.Unmarshal(%s, msg) failed with %v; want success", buf, err)
		return
	}
	if got, want := msg.GetNo().GetNote(), "golang"; got != want {
		t.Errorf("msg.GetNo().GetNote() = %q; want %q", got, want)
	}

	if value := resp.Header.Get("Content-Type"); value != contentType {
		t.Errorf("Content-Type was %s, wanted %s", value, contentType)
	}
}

func testEchoBody(t *testing.T, port int, apiPrefix string, useTrailers bool) {
	sent := examplepb.UnannotatedSimpleMessage{Id: "example"}
	payload, err := marshaler.Marshal(&sent)
	if err != nil {
		t.Fatalf("marshaler.Marshal(%#v) failed with %v; want success", payload, err)
	}

	apiURL := fmt.Sprintf("http://localhost:%d/%s/example/echo_body", port, apiPrefix)

	req, err := http.NewRequest("POST", apiURL, bytes.NewReader(payload))
	if err != nil {
		t.Errorf("http.NewRequest() failed with %v; want success", err)
		return
	}
	if useTrailers {
		req.Header.Set("TE", "trailers")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Errorf("client.Do(%v) failed with %v; want success", req, err)
		return
	}
	defer resp.Body.Close()
	buf, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Errorf("ioutil.ReadAll(resp.Body) failed with %v; want success", err)
		return
	}

	if got, want := resp.StatusCode, http.StatusOK; got != want {
		t.Errorf("resp.StatusCode = %d; want %d", got, want)
		t.Logf("%s", buf)
	}

	var received examplepb.UnannotatedSimpleMessage
	if err := marshaler.Unmarshal(buf, &received); err != nil {
		t.Errorf("marshaler.Unmarshal(%s, msg) failed with %v; want success", buf, err)
		return
	}
	if diff := cmp.Diff(&received, &sent, protocmp.Transform()); diff != "" {
		t.Errorf(diff)
	}

	if got, want := resp.Header.Get("Grpc-Metadata-Foo"), "foo1"; got != want {
		t.Errorf("Grpc-Metadata-Foo was %q, wanted %q", got, want)
	}
	if got, want := resp.Header.Get("Grpc-Metadata-Bar"), "bar1"; got != want {
		t.Errorf("Grpc-Metadata-Bar was %q, wanted %q", got, want)
	}

	wantedTrailers := map[bool]map[string]string{
		true: {
			"Grpc-Trailer-Foo": "foo2",
			"Grpc-Trailer-Bar": "bar2",
		},
		false: {},
	}

	for trailer, want := range wantedTrailers[useTrailers] {
		if got := resp.Trailer.Get(trailer); got != want {
			t.Errorf("%s was %q, wanted %q", trailer, got, want)
		}
	}
}

func testEchoValidationRules(t *testing.T, port int, contentType string) {
	sent := examplepb.ValidationRuleTestRequest{
		Id:  "example", // rules: NON_NIL, LEN_GT:2, LEN_LT: 61
		Num: 11,        // rules: GT:0
	}
	payload, err := marshaler.Marshal(&sent)
	if err != nil {
		t.Fatalf("marshaler.Marshal(%#v) failed with %v; want success", payload, err)
	}

	apiURL := fmt.Sprintf("http://localhost:%d/v1/example/echo:validationRules", port)
	resp, err := http.Post(apiURL, "", bytes.NewReader(payload))
	if err != nil {
		t.Errorf("http.Post(%q) failed with %v; want success", apiURL, err)
		return
	}
	defer resp.Body.Close()
	buf, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Errorf("ioutil.ReadAll(resp.Body) failed with %v; want success", err)
		return
	}

	if got, want := resp.StatusCode, http.StatusOK; got != want {
		t.Errorf("resp.StatusCode = %d; want %d", got, want)
		t.Logf("%s", buf)
	}

	//  Test validation error
	sent = examplepb.ValidationRuleTestRequest{
		Id:  "a", // rules: NON_NIL, LEN_GT:2, LEN_LT: 61
		Num: 0,   // rules: GT:0
	}
	payload, err = marshaler.Marshal(&sent)
	if err != nil {
		t.Fatalf("marshaler.Marshal(%#v) failed with %v; want success", payload, err)
	}

	apiURL = fmt.Sprintf("http://localhost:%d/v1/example/echo:validationRules", port)
	resp, err = http.Post(apiURL, "", bytes.NewReader(payload))
	if err != nil {
		t.Errorf("http.Post(%q) failed with %v; want success", apiURL, err)
		return
	}
	defer resp.Body.Close()
	buf, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Errorf("ioutil.ReadAll(resp.Body) failed with %v; want success", err)
		return
	}

	if got, want := resp.StatusCode, http.StatusBadRequest; got != want {
		t.Errorf("resp.StatusCode = %d; want %d", got, want)
		t.Logf("%s", buf)
	}

	if value := resp.Header.Get("Content-Type"); value != contentType {
		t.Errorf("Content-Type was %s, wanted %s", value, contentType)
	}
}
