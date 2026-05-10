# Demo Workspace Fixture

The generator creates a temporary git repository with:

- project `APP`
- ticket `APP-1`
- agent `builder-1`
- one active run normalized as `<RUN-ID>`
- one checkpoint evidence item
- one `test_result` evidence item
- one handoff to `agent:reviewer-1`
- a goal brief and goal manifest for `APP-1`

The temp workspace is deleted after generation. The checked-in examples contain only normalized output.
