package apply

import "testing"

func TestApplyPlanPartField(t *testing.T) {
	document := `{"version":1,"steps":[
		{"op":"add","title":"One","part":"2"},
		{"op":"rename","target":"10","title":"Eins","part":"Teil I — Einstieg"},
		{"op":"part","target":"20","value":"3"},
		{"op":"rename","target":"20","title":"Zwei","part":""}
	]}`
	steps, err := Parse([]byte(document))
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 4 {
		t.Fatalf("steps %d", len(steps))
	}
	if steps[0].Part == nil || *steps[0].Part != "2" {
		t.Fatalf("add part %+v", steps[0].Part)
	}
	if steps[1].Part == nil || *steps[1].Part != "Teil I — Einstieg" {
		t.Fatalf("rename part %+v", steps[1].Part)
	}
	if steps[2].Op != "part" || steps[2].Value != "3" || steps[2].Target != 20 {
		t.Fatalf("part step %+v", steps[2])
	}
	if steps[3].Part == nil || *steps[3].Part != "" {
		t.Fatalf("an explicit empty part must clear: %+v", steps[3].Part)
	}
	if steps[0].Part == steps[3].Part {
		t.Fatal("the steps must not share the pointer")
	}
}

func TestExecutePartSteps(t *testing.T) {
	service, _, paths := newScene(t)
	steps, err := Parse([]byte(`{"version":1,"steps":[
		{"op":"add","title":"Three","part":"2"},
		{"op":"rename","target":"10","title":"Eins"}
	]}`))
	if err != nil {
		t.Fatal(err)
	}
	receipts, err := service.Execute("fixture", steps, false)
	if err != nil {
		t.Fatal(err)
	}
	if receipts[0].Part != "Part II" {
		t.Fatalf("add receipt %+v", receipts[0])
	}
	manifest := manifestOf(t, paths)
	added, ok := manifest.FindNode(receipts[0].Target)
	if !ok || added.Part != "Part II" {
		t.Fatalf("added node %+v", added)
	}
	renamed, ok := manifest.FindNode(10)
	if !ok || renamed.Part != "" {
		t.Fatalf("a rename without part must keep the group: %+v", renamed)
	}
}
