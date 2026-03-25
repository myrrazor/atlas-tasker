package markdown

import "testing"

func FuzzDecodeTicketMarkdown(f *testing.F) {
	for _, seed := range []string{
		"---\nid: APP-1\nproject: APP\ntitle: Test\ntype: task\nstatus: backlog\npriority: medium\ncreated_at: 2026-03-24T00:00:00Z\nupdated_at: 2026-03-24T00:00:00Z\nschema_version: 2\nlabels: []\nblocked_by: []\nblocks: []\n---\n\n# Summary\n\nhi\n\n## Description\n\nbody\n\n## Acceptance Criteria\n\n- one\n\n## Notes\n\nnone\n",
		"---\nbad\n---\nbody",
		"",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, doc string) {
		_, _ = DecodeTicketMarkdown(doc)
	})
}
