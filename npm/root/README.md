# @aibo666/ebo

This npm package is a small launcher for the native Ebo CLI binary. The core runtime is implemented in Go and distributed through platform-specific optional packages.

```bash
npm install --global @aibo666/ebo
ebo version
```

Agent runtimes can use the same installed CLI as a lightweight pre-write hook:

```bash
ebo hook pre-write --path path/to/file --json
```

The command is local and deterministic: it does not call AI or access the network. Use `ebo hooks install` separately to install the optional Git pre-commit guard for the current repository.

Codex users can explicitly install a project-local native `PreToolUse` adapter from an initialized Ebo project:

```bash
ebo hooks install codex
```

This merges the hook into the current project's `.codex/hooks.json`; it never writes to the user's `~/.codex/hooks.json`. Then open `/hooks` in Codex to review and trust the new hook definition.

Installing this npm package does not modify Codex configuration or Git hooks. The launcher does not download binaries during `postinstall`; integrations are installed only by explicit `ebo hooks install ...` commands inside an initialized project.

The MVP npm distribution supports Windows x64 and macOS arm64. Other platforms will fail with a clear unsupported-platform message.
