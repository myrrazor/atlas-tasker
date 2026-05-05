package tui

import "testing"

func BenchmarkModelStartupAndRefresh(b *testing.B) {
	for i := 0; i < b.N; i++ {
		root := seededTUIWorkspaceForBench(b)
		m, err := newModel(root, "")
		if err != nil {
			b.Fatalf("new model: %v", err)
		}
		msg := m.refresh()().(loadedMsg)
		_, _ = m.Update(msg)
		m.close()
	}
}

func seededTUIWorkspaceForBench(b *testing.B) string {
	b.Helper()
	root := b.TempDir()
	seededTUIWorkspaceAt(root, func(format string, args ...any) {
		b.Fatalf(format, args...)
	})
	return root
}
