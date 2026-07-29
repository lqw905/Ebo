# npm Packaging

The npm distribution is intentionally thin:

- `npm/root` publishes `@aibo666/ebo` and registers the `ebo` command.
- `npm/platforms/*` publish supported platform-specific packages containing native Go binaries.
- The root package chooses the matching optional dependency at runtime.
- There is no `postinstall` download step.

The MVP publishes only Windows x64 and macOS arm64 packages:

```text
@aibo666/ebo-win32-x64
@aibo666/ebo-darwin-arm64
```

Release automation should copy built binaries into each supported platform package before publishing.
