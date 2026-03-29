package contracts

import "testing"

func TestSavedViewValidate(t *testing.T) {
	valid := SavedView{
		Name:  "ready-work",
		Kind:  SavedViewKindSearch,
		Query: "status=ready",
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid saved view, got %v", err)
	}

	cases := []SavedView{
		{Name: "", Kind: SavedViewKindBoard},
		{Name: "bad-kind", Kind: SavedViewKind("wat")},
		{Name: "bad-query", Kind: SavedViewKindSearch, Query: "wat"},
		{Name: "bad-actor", Kind: SavedViewKindQueue, Actor: Actor("nope")},
		{Name: "bad-column", Kind: SavedViewKindBoard, Board: SavedBoardConfig{Columns: []Status{"wat"}}},
	}
	for _, tc := range cases {
		if err := tc.Validate(); err == nil {
			t.Fatalf("expected validation failure for %#v", tc)
		}
	}
}
