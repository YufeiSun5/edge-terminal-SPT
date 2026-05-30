package discovery

import "testing"

func TestDiscoverJSONLeavesAndDeduplicates(t *testing.T) {
	worker := &Worker{}
	tags, err := worker.discover(1, "topic", []byte(`{"supply":{"temp":23.5},"status":{"running":true},"items":[{"name":"A"}],"nil":null}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 3 {
		t.Fatalf("unexpected tags: %+v", tags)
	}
	again, err := worker.discover(1, "topic", []byte(`{"supply":{"temp":24}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 0 {
		t.Fatalf("expected duplicate discovery to be skipped: %+v", again)
	}
	if tags[0].VarID == 0 || tags[0].GatewayID != 1 || !tags[0].Discovered || tags[0].Enabled {
		t.Fatalf("unexpected tag fields: %+v", tags[0])
	}
}

func TestDiscoverKIOAndHelpers(t *testing.T) {
	worker := &Worker{}
	tags, err := worker.discover(2, "kio", []byte(`{"Objs":[{"N":"A-B C","1":1},{"N":"Flag","1":false}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 2 || tags[0].VarName != "A_B_C" || tags[1].DataType != "BOOL" {
		t.Fatalf("unexpected kio tags: %+v", tags)
	}
	if sanitizeVarName("") != "unnamed" || lastPathPart("a.b.c") != "c" || inferDataType("x") != "STRING" {
		t.Fatal("helper behavior failed")
	}
	idA := stableVarID(1, "a")
	if idA != stableVarID(1, "a") || idA == stableVarID(2, "a") {
		t.Fatal("stable var id behavior failed")
	}
}

func TestDiscoverRejectsInvalidJSON(t *testing.T) {
	worker := &Worker{}
	if _, err := worker.discover(1, "topic", []byte(`bad`)); err == nil {
		t.Fatal("expected invalid json")
	}
}
