# npm Packaging

The npm distribution is intentionally thin:

- `npm/root` publishes `@lqw905/ebo` and registers the `ebo` command.
- `npm/platforms/*` publish platform-specific packages containing native Go binaries.
- The root package chooses the matching optional dependency at runtime.
- There is no `postinstall` download step.

Release automation should copy built binaries into each platform package before publishing.
