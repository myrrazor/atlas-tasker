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

func BenchmarkPanelRendering(b *testing.B) {
	root := seededTUIWorkspaceForBench(b)
	m, err := newModel(root, "")
	if err != nil {
		b.Fatalf("new model: %v", err)
	}
	defer m.close()
	msg := m.refresh()().(loadedMsg)
	updated, _ := m.Update(msg)
	m = updated.(model)

	screens := []screen{screenBoard, screenDetail, screenInbox, screenOps}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.screen = screens[i%len(screens)]
		_ = m.View()
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
