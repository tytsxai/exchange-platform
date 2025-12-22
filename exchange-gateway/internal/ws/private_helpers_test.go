package ws

import "testing"

func TestQueryHelpers(t *testing.T) {
	query := map[string][]string{
		"b": {"2", "1"},
		"a": {"3"},
	}
	canonical := canonicalQuery(query)
	if canonical != "a=3&b=1&b=2" {
		t.Fatalf("canonical query = %s", canonical)
	}

	built := buildCanonicalString(100, "n1", "get", "/ws/private", query)
	expected := "100\nn1\nGET\n/ws/private\na=3&b=1&b=2"
	if built != expected {
		t.Fatalf("canonical string = %s", built)
	}

	signed := sign("s", "data")
	if signed != "2ce1c84c161c51af83642a7785795d4f1c407cf19b5f8b51b39ef5774ee28dba" {
		t.Fatalf("signature = %s", signed)
	}
}

func TestCloneQueryWithoutSignature(t *testing.T) {
	original := map[string][]string{
		querySignature: {"sig"},
		"foo":          {"bar"},
	}
	cloned := cloneQueryWithoutSignature(original)
	if _, ok := cloned[querySignature]; ok {
		t.Fatal("expected signature to be removed")
	}
	if got := cloned["foo"]; len(got) != 1 || got[0] != "bar" {
		t.Fatalf("expected foo=bar, got %v", got)
	}

	empty := cloneQueryWithoutSignature(map[string][]string{})
	if len(empty) != 0 {
		t.Fatal("expected empty clone for empty query")
	}
}
