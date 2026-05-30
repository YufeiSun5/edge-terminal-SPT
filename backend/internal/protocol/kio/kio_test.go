package kio

import (
	"encoding/json"
	"testing"

	"github.com/tidwall/gjson"
)

func TestParseUpdates(t *testing.T) {
	payload := []byte(`{
		"Writer":"sa",
		"PVs":{"2":"2026-05-27 10:11:12.000 +0800","3":192},
		"Objs":[
			{"N":"1号机开始检测","1":2},
			{"N":"压力","1":12.5,"3":0}
		]
	}`)

	updates, err := ParseUpdates(payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(updates) != 2 {
		t.Fatalf("updates len=%d", len(updates))
	}
	if updates[0].Name != "1号机开始检测" || updates[0].Value.Int() != 2 || updates[0].Quality != 1 {
		t.Fatalf("unexpected first update: %+v", updates[0])
	}
	if updates[1].DataType != "FLOAT" || updates[1].Quality != 0 {
		t.Fatalf("unexpected second update: %+v", updates[1])
	}
}

func TestExtractValueWithPVsFallback(t *testing.T) {
	payload := `{"PVs":{"1":7,"3":192},"Objs":[{"N":"控制变量"}]}`
	value, quality := ExtractValue(payload, "控制变量", `Objs.#(N=="控制变量").1`)
	if !value.Exists() || value.Int() != 7 {
		t.Fatalf("unexpected value: exists=%v value=%s", value.Exists(), value.Raw)
	}
	if quality != 1 {
		t.Fatalf("quality=%d", quality)
	}
}

func TestExtractValueWithPVsFallbackUsesRawJSONPath(t *testing.T) {
	payload := `{"PVs":{"1":0,"3":192},"Objs":[{"N":"$ProjectControlOfS7-1200"}]}`
	value, quality := ExtractValue(payload, "$ProjectControlOfS7_1200", `Objs.#(N=="$ProjectControlOfS7-1200").1`)
	if !value.Exists() || value.Int() != 0 {
		t.Fatalf("unexpected value: exists=%v value=%s", value.Exists(), value.Raw)
	}
	if quality != 1 {
		t.Fatalf("quality=%d", quality)
	}
}

func TestBuildWritePayload(t *testing.T) {
	payload, err := BuildWritePayload(WriteRequest{
		Writer:   "edge",
		Username: "sa",
		Password: "secret",
		QID:      123,
		Values: []WriteValue{
			{Name: "1号机开始检测", Value: 1},
			{Name: "检测批次", Value: "T-001"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	var decoded WritePayload
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.QID != 123 || decoded.PNs[ValueKey] == "" || len(decoded.Objs) != 2 {
		t.Fatalf("unexpected payload: %+v", decoded)
	}
	if decoded.Objs[0]["N"] != "1号机开始检测" || decoded.Objs[0][ValueKey].(float64) != 1 {
		t.Fatalf("unexpected object: %+v", decoded.Objs[0])
	}
}

func TestBuildWritePayloadGeneratedQIDStaysInKIOFriendlyRange(t *testing.T) {
	payload, err := BuildWritePayload(WriteRequest{
		Writer: "edge",
		Values: []WriteValue{{Name: "台1_39", Value: 13.41}},
	})
	if err != nil {
		t.Fatal(err)
	}
	qid := QIDFromPayload(payload)
	if qid <= 0 || qid >= 1_000_000_000 {
		t.Fatalf("unexpected generated qid: %d", qid)
	}
}

func TestParseWriteAck(t *testing.T) {
	ack, ok, err := ParseWriteAck([]byte(`{"Qid":123,"ProcessStep":100,"Result":"ok","Time":"2021-02-03 13:49:13.842"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !ok || ack.QID != 123 || ack.ProcessStep != 100 || ack.Result != "ok" || !ack.Success {
		t.Fatalf("unexpected ack: ok=%v ack=%+v", ok, ack)
	}
}

func TestParseWriteAckPartialIsNotSuccess(t *testing.T) {
	ack, ok, err := ParseWriteAck([]byte(`{"Qid":123,"ProcessStep":100,"Result":"some ok"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !ok || ack.Success {
		t.Fatalf("unexpected ack: ok=%v ack=%+v", ok, ack)
	}
}

func TestTopics(t *testing.T) {
	if SetDataTopic("S_KIO_Project") != "setdata_S_KIO_Project" {
		t.Fatal("bad setdata topic")
	}
	if SetDataResultTopic("S_KIO_Project", "edge") != "setdata_result_S_KIO_Project_edge" {
		t.Fatal("bad result topic")
	}
	if QueryTopic("S_KIO_Project") != "query_S_KIO_Project" {
		t.Fatal("bad query topic")
	}
	if QueryResultTopic("S_KIO_Project", "edge") != "query_result_S_KIO_Project_edge" {
		t.Fatal("bad query result topic")
	}
	if QueryAllTagsTopic("S_KIO_Project") != "Query_AllKIOTags_S_KIO_Project" {
		t.Fatal("bad query all topic")
	}
}

func TestSourceTimeOffsetFromPVs(t *testing.T) {
	updates, err := ParseUpdates([]byte(`{
		"PVs":{"1":0,"2":"2019-03-01 08:00:00.000","3":192},
		"Objs":[{"N":"Tag1","1":1,"2":1500}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(updates) != 1 {
		t.Fatalf("updates len=%d", len(updates))
	}
	if updates[0].SourceTime.Format("15:04:05.000") != "08:00:01.500" {
		t.Fatalf("unexpected source time: %s", updates[0].SourceTime.Format("15:04:05.000"))
	}
}

func TestDiscoverVariablesDeduplicatesAndLooksLikePayload(t *testing.T) {
	payload := []byte(`{"Objs":[{"N":"A","1":1},{"N":"A","1":2},{"N":"B","1":true}]}`)
	if !LooksLikePayload(payload) || LooksLikePayload([]byte(`{"items":[]}`)) {
		t.Fatal("looks-like payload detection failed")
	}
	defs, err := DiscoverVariables(payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(defs) != 2 || defs[0].DataType != "INT" || defs[1].DataType != "BOOL" {
		t.Fatalf("unexpected defs: %+v", defs)
	}
}

func TestParseUpdatesSkipsMissingNamesAndValues(t *testing.T) {
	updates, err := ParseUpdates([]byte(`{"Objs":[{"1":1},{"N":"missing"},{"N":"nil","1":null},{"N":"ok","1":"text"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(updates) != 1 || updates[0].Name != "ok" || updates[0].DataType != "STRING" {
		t.Fatalf("unexpected updates: %+v", updates)
	}
	if _, err := ParseUpdates([]byte(`not-json`)); err == nil {
		t.Fatal("expected invalid json error")
	}
}

func TestBuildWritePayloadValidationAndCustomKeys(t *testing.T) {
	if _, err := BuildWritePayload(WriteRequest{}); err == nil {
		t.Fatal("expected writer validation error")
	}
	if _, err := BuildWritePayload(WriteRequest{Writer: "edge"}); err == nil {
		t.Fatal("expected values validation error")
	}
	if _, err := BuildWritePayload(WriteRequest{Writer: "edge", Values: []WriteValue{{Value: 1}}}); err == nil {
		t.Fatal("expected value name validation error")
	}

	payload, err := BuildWritePayload(WriteRequest{
		Writer: "edge",
		Values: []WriteValue{
			{Name: "custom", Key: "9", Value: true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var decoded WritePayload
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.PNs["9"] != "B" || decoded.Objs[0]["9"] != true || QIDFromPayload(payload) == 0 {
		t.Fatalf("unexpected custom payload: %+v", decoded)
	}
}

func TestParseWriteAckVariants(t *testing.T) {
	if _, _, err := ParseWriteAck([]byte(`bad`)); err == nil {
		t.Fatal("expected invalid json")
	}
	if _, ok, err := ParseWriteAck([]byte(`{"Result":"ok"}`)); err != nil || ok {
		t.Fatalf("expected no qid: ok=%v err=%v", ok, err)
	}
	ack, ok, err := ParseWriteAck([]byte(`{"qid":5,"ProcessStep":99,"error":"doing"}`))
	if err != nil || !ok || ack.QID != 5 || ack.Message != "doing" || ack.Success {
		t.Fatalf("unexpected ack: ok=%v ack=%+v err=%v", ok, ack, err)
	}
}

func TestSourceTimeAndInferDataTypeVariants(t *testing.T) {
	root := gjsonParse(`{"PVs":{"2":1710000000001},"WriteTime":"2026-05-29T12:00:00Z"}`)
	if at := SourceTimeFrom(gjsonParse(`{}`), root); at.IsZero() {
		t.Fatal("expected unix millis source time")
	}
	if InferDataType(gjsonParse(`1.5`)) != "FLOAT" || InferDataType(gjsonParse(`false`)) != "BOOL" {
		t.Fatal("infer type failed")
	}
	if resultIsSuccess(gjsonParse(`"success"`)) != true || resultIsSuccess(gjsonParse(`0`)) != false {
		t.Fatal("success result parsing failed")
	}
}

func gjsonParse(raw string) gjson.Result {
	return gjson.Parse(raw)
}
