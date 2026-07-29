# @lqw905/ebo

This npm package is a small launcher for the native Ebo CLI binary. The core runtime is implemented in Go and distributed through platform-specific optional packages.

```bash
npm install --global @lqw905/ebo
ebo version
```

The launcher does not download binaries during `postinstall`.

The MVP npm distribution supports Windows x64 and macOS arm64. Other platforms will fail with a clear unsupported-platform message.
