package render

import "testing"

// These two tests prove the preserved fixed-byte request guards for single and mixed payloads.
func TestBuildRequestSpecBytesFixedSingleFieldValidation(t *testing.T) {
	params, p1Expr, p2Expr, dataPrepLines, dataExpr, hasData, err := buildRequestSpec("setHash", &Message{
		Fields: []Field{
			{
				Name:        "hash",
				Type:        FieldTypeBytesFixed,
				FixedLength: 32,
				Location:    ParameterLocationData,
			},
		},
	})
	if err != nil {
		t.Fatalf("buildRequestSpec returned error: %v", err)
	}

	if len(params) != 1 {
		t.Fatalf("params length mismatch: got %d want 1", len(params))
	}
	if params[0].Name != "hash" || params[0].Type != "Data" {
		t.Fatalf("parameter mismatch: got %#v", params[0])
	}
	if p1Expr != "0x00" || p2Expr != "0x00" {
		t.Fatalf("p1/p2 mismatch: got p1=%q p2=%q", p1Expr, p2Expr)
	}
	if !hasData {
		t.Fatalf("hasData mismatch: got false want true")
	}
	if dataExpr != "hash" {
		t.Fatalf("dataExpr mismatch: got %q want %q", dataExpr, "hash")
	}
	wantLines := []string{
		"if hash.count != 32 { throw TransportError.invalidResponse }",
	}
	if !equalStrings(dataPrepLines, wantLines) {
		t.Fatalf("dataPrepLines mismatch:\nwant: %#v\ngot:  %#v", wantLines, dataPrepLines)
	}
}

func TestBuildRequestSpecBytesFixedMultiFieldValidation(t *testing.T) {
	params, p1Expr, p2Expr, dataPrepLines, dataExpr, hasData, err := buildRequestSpec("pack", &Message{
		Fields: []Field{
			{Name: "prefix", Type: FieldTypeU8, Location: ParameterLocationData},
			{Name: "hash", Type: FieldTypeBytesFixed, FixedLength: 32, Location: ParameterLocationData},
		},
	})
	if err != nil {
		t.Fatalf("buildRequestSpec returned error: %v", err)
	}

	if len(params) != 2 {
		t.Fatalf("params length mismatch: got %d want 2", len(params))
	}
	if p1Expr != "0x00" || p2Expr != "0x00" {
		t.Fatalf("p1/p2 mismatch: got p1=%q p2=%q", p1Expr, p2Expr)
	}
	if !hasData {
		t.Fatalf("hasData mismatch: got false want true")
	}
	if dataExpr != "data" {
		t.Fatalf("dataExpr mismatch: got %q want %q", dataExpr, "data")
	}
	wantLines := []string{
		"var data = Data()",
		"data.append(prefix)",
		"if hash.count != 32 { throw TransportError.invalidResponse }",
		"data.append(hash)",
	}
	if !equalStrings(dataPrepLines, wantLines) {
		t.Fatalf("dataPrepLines mismatch:\nwant: %#v\ngot:  %#v", wantLines, dataPrepLines)
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
