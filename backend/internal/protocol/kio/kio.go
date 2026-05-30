package kio

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/tidwall/gjson"
)

const (
	ValueKey       = "1"
	TimeKey        = "2"
	QualityKey     = "3"
	GoodQualityRaw = 192
)

var writeQIDCounter uint64

type VariableUpdate struct {
	Name       string
	SourcePath string
	JSONPath   string
	DataType   string
	Value      gjson.Result
	Quality    int
	SourceTime time.Time
}

type VariableDefinition struct {
	Name       string
	SourcePath string
	JSONPath   string
	DataType   string
}

type WriteValue struct {
	Name  string      `json:"name"`
	Key   string      `json:"key,omitempty"`
	Value interface{} `json:"value"`
}

type WriteRequest struct {
	Writer    string       `json:"Writer"`
	WriteTime string       `json:"WriteTime,omitempty"`
	Username  string       `json:"Username"`
	Password  string       `json:"Password"`
	QID       int64        `json:"Qid"`
	Values    []WriteValue `json:"values"`
}

type WritePayload struct {
	Writer    string                   `json:"Writer"`
	WriteTime string                   `json:"WriteTime"`
	Username  string                   `json:"Username,omitempty"`
	Password  string                   `json:"Password,omitempty"`
	QID       int64                    `json:"Qid"`
	PNs       map[string]string        `json:"PNs"`
	PVs       map[string]interface{}   `json:"PVs"`
	Objs      []map[string]interface{} `json:"Objs"`
}

type WriteAck struct {
	QID         int64
	ProcessStep int
	Result      string
	Time        string
	Success     bool
	Message     string
}

func SetDataTopic(clientID string) string {
	return "setdata_" + clientID
}

func SetDataResultTopic(clientID string, writer string) string {
	return "setdata_result_" + clientID + "_" + writer
}

func QueryTopic(clientID string) string {
	return "query_" + clientID
}

func QueryResultTopic(clientID string, whoQ string) string {
	return "query_result_" + clientID + "_" + whoQ
}

func QueryAllTagsTopic(clientID string) string {
	return "Query_AllKIOTags_" + clientID
}

func LooksLikePayload(payload []byte) bool {
	root := gjson.ParseBytes(payload)
	return root.Get("Objs").IsArray()
}

func ParseUpdates(payload []byte) ([]VariableUpdate, error) {
	if !gjson.ValidBytes(payload) {
		return nil, fmt.Errorf("invalid json")
	}

	root := gjson.ParseBytes(payload)
	objs := root.Get("Objs").Array()
	updates := make([]VariableUpdate, 0, len(objs))
	for _, obj := range objs {
		name := obj.Get("N").String()
		if name == "" {
			continue
		}
		value := obj.Get(ValueKey)
		if !value.Exists() && root.Get("PVs").Exists() {
			value = root.Get("PVs." + ValueKey)
		}
		if !value.Exists() || value.Type == gjson.Null {
			continue
		}

		updates = append(updates, VariableUpdate{
			Name:       name,
			SourcePath: PathFor(name, ValueKey),
			JSONPath:   PathFor(name, ValueKey),
			DataType:   InferDataType(value),
			Value:      value,
			Quality:    QualityFrom(obj, root),
			SourceTime: SourceTimeFrom(obj, root),
		})
	}
	return updates, nil
}

func DiscoverVariables(payload []byte) ([]VariableDefinition, error) {
	updates, err := ParseUpdates(payload)
	if err != nil {
		return nil, err
	}
	defs := make([]VariableDefinition, 0, len(updates))
	seen := make(map[string]struct{}, len(updates))
	for _, update := range updates {
		if _, ok := seen[update.Name]; ok {
			continue
		}
		seen[update.Name] = struct{}{}
		defs = append(defs, VariableDefinition{
			Name:       update.Name,
			SourcePath: update.SourcePath,
			JSONPath:   update.JSONPath,
			DataType:   update.DataType,
		})
	}
	return defs, nil
}

func ExtractValue(payload string, varName string, jsonPath string) (gjson.Result, int) {
	root := gjson.Parse(payload)
	result := root.Get(jsonPath)
	quality := 1

	if !root.Get("PVs").Exists() {
		return result, quality
	}

	objectPath := jsonPath
	if idx := strings.LastIndex(objectPath, "."); idx > 0 {
		objectPath = objectPath[:idx]
	}

	if !result.Exists() && (root.Get(objectPath).Exists() || root.Get(`Objs.#(N=="`+escapeQueryString(varName)+`")`).Exists()) {
		propertyKey := jsonPath
		if idx := strings.LastIndex(propertyKey, "."); idx >= 0 {
			propertyKey = propertyKey[idx+1:]
		}
		result = root.Get("PVs." + propertyKey)
	}

	qualityCode := root.Get(objectPath + "." + QualityKey)
	if !qualityCode.Exists() {
		qualityCode = root.Get("PVs." + QualityKey)
	}
	if qualityCode.Exists() && qualityCode.Int() != GoodQualityRaw {
		quality = 0
	}

	return result, quality
}

func BuildWritePayload(req WriteRequest) ([]byte, error) {
	if req.Writer == "" {
		return nil, fmt.Errorf("writer is required")
	}
	if req.QID == 0 {
		req.QID = nextWriteQID()
	}
	if len(req.Values) == 0 {
		return nil, fmt.Errorf("values is required")
	}

	writeTime := req.WriteTime
	if writeTime == "" {
		writeTime = time.Now().Format("2006-01-02 15:04:05.000 -0700")
	}

	pns := map[string]string{
		ValueKey:   "V",
		TimeKey:    "T",
		QualityKey: "Q",
		"4":        "F",
		"5":        "S",
	}
	pvs := map[string]interface{}{
		ValueKey:   0,
		TimeKey:    writeTime,
		QualityKey: 0,
	}
	objs := make([]map[string]interface{}, 0, len(req.Values))
	for _, item := range req.Values {
		if item.Name == "" {
			return nil, fmt.Errorf("value name is required")
		}
		key := item.Key
		if key == "" {
			key = ValueKey
		}
		if key != ValueKey {
			pns[key] = inferPNType(item.Value)
		}
		objs = append(objs, map[string]interface{}{
			"N": item.Name,
			key: item.Value,
		})
	}

	payload := WritePayload{
		Writer:    req.Writer,
		WriteTime: writeTime,
		Username:  req.Username,
		Password:  req.Password,
		QID:       req.QID,
		PNs:       pns,
		PVs:       pvs,
		Objs:      objs,
	}
	return json.Marshal(payload)
}

func nextWriteQID() int64 {
	base := time.Now().UnixMilli() % 1_000_000
	seq := atomic.AddUint64(&writeQIDCounter, 1) % 1000
	qid := base*1000 + int64(seq)
	if qid <= 0 {
		return int64(seq + 1)
	}
	return qid
}

func ParseWriteAck(payload []byte) (WriteAck, bool, error) {
	if !gjson.ValidBytes(payload) {
		return WriteAck{}, false, fmt.Errorf("invalid json")
	}
	root := gjson.ParseBytes(payload)
	qid := root.Get("Qid")
	if !qid.Exists() {
		qid = root.Get("qid")
	}
	if !qid.Exists() {
		return WriteAck{}, false, nil
	}

	successResult := firstExisting(root, "Success", "success", "OK", "ok", "Result", "result")
	ack := WriteAck{
		QID:         qid.Int(),
		ProcessStep: int(root.Get("ProcessStep").Int()),
		Result:      successResult.String(),
		Time:        firstExisting(root, "Time", "time").String(),
	}
	ack.Success = ack.ProcessStep == 100 && strings.EqualFold(strings.TrimSpace(ack.Result), "ok")
	message := firstExisting(root, "Message", "message", "Msg", "msg", "Error", "error")
	if message.Exists() {
		ack.Message = message.String()
	} else {
		ack.Message = ack.Result
	}
	return ack, true, nil
}

func QIDFromPayload(payload []byte) int64 {
	qid := gjson.GetBytes(payload, "Qid")
	if !qid.Exists() {
		qid = gjson.GetBytes(payload, "qid")
	}
	return qid.Int()
}

func PathFor(name string, key string) string {
	if key == "" {
		key = ValueKey
	}
	return `Objs.#(N=="` + escapeQueryString(name) + `").` + key
}

func QualityFrom(obj gjson.Result, root gjson.Result) int {
	qualityCode := obj.Get(QualityKey)
	if !qualityCode.Exists() {
		qualityCode = root.Get("PVs." + QualityKey)
	}
	if qualityCode.Exists() && qualityCode.Int() != GoodQualityRaw {
		return 0
	}
	return 1
}

func SourceTimeFrom(obj gjson.Result, root gjson.Result) time.Time {
	objTime := obj.Get(TimeKey)
	baseTime := root.Get("PVs." + TimeKey)
	if objTime.Exists() && objTime.Type == gjson.Number && baseTime.Exists() {
		if base, ok := parseTimeResult(baseTime); ok {
			return base.Add(time.Duration(objTime.Int()) * time.Millisecond)
		}
	}

	candidates := []gjson.Result{obj.Get(TimeKey), root.Get("PVs." + TimeKey), root.Get("WriteTime")}
	for _, candidate := range candidates {
		if !candidate.Exists() {
			continue
		}
		if at, ok := parseTimeResult(candidate); ok {
			return at
		}
	}
	return time.Time{}
}

func InferDataType(value gjson.Result) string {
	switch value.Type {
	case gjson.True, gjson.False:
		return "BOOL"
	case gjson.Number:
		raw := value.Raw
		if !strings.ContainsAny(raw, ".eE") {
			return "INT"
		}
		return "FLOAT"
	case gjson.String:
		return "STRING"
	default:
		return "STRING"
	}
}

func firstExisting(root gjson.Result, paths ...string) gjson.Result {
	for _, path := range paths {
		result := root.Get(path)
		if result.Exists() {
			return result
		}
	}
	return gjson.Result{}
}

func resultIsSuccess(result gjson.Result) bool {
	if !result.Exists() {
		return false
	}
	switch result.Type {
	case gjson.True:
		return true
	case gjson.False:
		return false
	case gjson.Number:
		return result.Int() == 1 || result.Int() == GoodQualityRaw
	case gjson.String:
		switch strings.ToLower(strings.TrimSpace(result.String())) {
		case "1", "true", "ok", "success", "succeeded":
			return true
		default:
			return false
		}
	default:
		return false
	}
}

func parseTimeResult(result gjson.Result) (time.Time, bool) {
	if result.Type == gjson.Number {
		raw := result.Int()
		if raw <= 0 {
			return time.Time{}, false
		}
		if raw > 1_000_000_000_000 {
			return time.UnixMilli(raw), true
		}
		return time.Unix(raw, 0), true
	}

	text := strings.TrimSpace(result.String())
	if text == "" {
		return time.Time{}, false
	}
	layouts := []struct {
		value string
		local bool
	}{
		{value: "2006-01-02 15:04:05.000 -0700"},
		{value: "2006-01-02 15:04:05 -0700"},
		{value: "2006-01-02 15:04:05.000", local: true},
		{value: "2006-01-02 15:04:05", local: true},
		{value: time.RFC3339Nano},
		{value: time.RFC3339},
	}
	for _, layout := range layouts {
		var (
			at  time.Time
			err error
		)
		if layout.local {
			at, err = time.ParseInLocation(layout.value, text, time.Local)
		} else {
			at, err = time.Parse(layout.value, text)
		}
		if err == nil {
			return at, true
		}
	}
	return time.Time{}, false
}

func inferPNType(value interface{}) string {
	switch typed := value.(type) {
	case float32, float64:
		return "F"
	case string:
		return "S"
	case bool:
		return "B"
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return "V"
	case json.Number:
		if _, err := strconv.ParseInt(string(typed), 10, 64); err == nil {
			return "V"
		}
		return "F"
	default:
		return "V"
	}
}

func escapeQueryString(value string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value)
}
