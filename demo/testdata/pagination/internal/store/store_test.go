package store

import "testing"

func TestListPagination(t *testing.T) {
	s := New()
	for _, title := range []string{"a", "b", "c", "d", "e"} {
		s.Add(title)
	}
	tests := []struct {
		offset, limit int
		want          []string
	}{
		{0, 2, []string{"a", "b"}},
		{2, 2, []string{"c", "d"}},
		{4, 2, []string{"e"}},
		{10, 2, nil},
	}
	for _, tt := range tests {
		got, total := s.List(tt.offset, tt.limit)
		if total != 5 {
			t.Errorf("total = %d, want 5", total)
		}
		if len(got) != len(tt.want) {
			t.Fatalf("List(%d, %d) = %d items, want %d", tt.offset, tt.limit, len(got), len(tt.want))
		}
		for i, todo := range got {
			if todo.Title != tt.want[i] {
				t.Errorf("List(%d, %d)[%d] = %q, want %q", tt.offset, tt.limit, i, todo.Title, tt.want[i])
			}
		}
	}
}
