# Ebo

Ebo 是一个本地 CLI Runtime，用于维护软件项目的 Prompt Tree（项目意图树）。它不调用 AI API、不生成 Prompt、也不自行执行代码改动。人或外部 Agent 编写 Prompt Markdown；Ebo 负责暂存、审阅、审批、应用、校验和扫描这棵树。

当前仓库包含第一个 MVP 切片。

## 项目隔离

`ebo init` 把项目意图收敛在 `.ebo/` 命名空间下，而不是把 Prompt 文件散落在应用源码目录中：

```text
.ebo/
  WORKFLOW.md  中文使用手册和可复制的 Agent 提示词
  tree/        正式、受版本控制的 Prompt Tree
  proposals/   等待人工审阅的 Prompt 变更
  plans/       执行计划
  receipts/    执行凭证
  runtime/     生成的上下文与导入
  cache/       忽略的本地缓存
  locks/       忽略的进程锁
  tmp/         忽略的临时文件
```

它还会生成一份中文的 `.ebo/WORKFLOW.md`，内含面向每个用户与 Agent 流程的可复制提示词。`AGENTS.md`（以及在请求时生成的 `CLAUDE.md`）中的一小段受管区块会告诉 Agent：动手前先阅读这份文档。只有通过了哈希绑定的人工审批的 Proposal 才能进入正式树、成为可执行工作。

Ebo 掌握执行决策，因此 Agent 不需要加载整棵 Prompt Tree。`project.root`、未审批的 Prompt、已满足的哈希、依赖未就绪的 Prompt 都会被跳过。失败或阻塞的 Prompt 可以重试。`ebo next` 返回一个可执行任务后，Agent 只需用 `ebo context <prompt-id> --depth 0` 加载被选中的 Prompt 及其直接语义链接。更广的层级上下文按需启用，以降低不必要的工作量与上下文消耗。

## 构建

```bash
go test ./...
go build -o ebo ./cmd/ebo
```

## CI

GitHub Actions 在 Windows、macOS、Linux 上运行：

```text
go test ./...
go build ./cmd/ebo
go test ./internal/cli -run TestCLISmoke -count=1
node scripts/check-npm-packages.mjs
npm pack --dry-run ./npm/root
```

npm CI 作业还会用假二进制运行一次打包冒烟测试，验证受支持平台包的布局。

## 已实现命令

```bash
ebo init --agents codex,claude
ebo doctor
ebo status
ebo config get
ebo mode [strict|silent]

ebo add --stdin
ebo add --file <path>
ebo add --dir <path>
ebo add --dry-run --file <path>
ebo review [proposal-id]
ebo approve <proposal-id>
ebo reject <proposal-id> --reason "..."
ebo apply <proposal-id>

ebo tree list
ebo tree show <node-id>
ebo tree validate
ebo tree search "<text>"
ebo tree graph [node-id]
ebo tree graph --around <node-id>
ebo context <node-id> --depth 2 --out .ebo/runtime/context.json

ebo scan [node-id]
ebo plan [node-id]
ebo plan list
ebo plan show <plan-id>
ebo next [plan-id]
ebo export <plan-id> --format markdown
ebo export <plan-id> --format json
ebo report <task-id> --plan <plan-id> --result passed --note "..."
ebo verify <plan-id>
ebo abort <plan-id>
ebo commit <plan-id> --dry-run
ebo import . --out .ebo/runtime/import
ebo lock status
ebo guard check
ebo guard check --staged
ebo hook pre-write --path <file>
ebo hook pre-write --path <file> --json
ebo hook codex-pre-tool-use
ebo hooks install
ebo hooks status
ebo hooks install codex
ebo hooks status codex
```

`approve` 有意要求交互终端和 `[y/N]` 确认。Ebo 在内部绑定完整 Proposal 哈希，并在 `apply` 时再次校验，因此用户无需抄写 SHA-256 值，而内容一旦变化仍会被拒绝。

## 治理模式

Ebo 支持两种项目治理模式：

- `strict`（严格，默认）：每个 Prompt 在进入正式树前都需要交互式人工审批。Agent 会停下来请求用户运行 `ebo approve`。
- `silent`（静默）：Agent 自行运行 `add → approve → apply → next → report`，不再暂停。只有人类可以切换模式。

`ebo mode` 显示当前模式；`ebo mode strict|silent` 切换模式。切换总是要求交互终端与 `[y/N]` 确认，因此 Agent 永远无法放松自己的审批门——这个出口只属于终端前的人。在 `silent` 模式下，`ebo approve` 仍会校验 Proposal 哈希，fail-closed 的执行门与 scope 检查仍然生效；被跳过的只有逐条的人工确认。切换模式会重新生成受管的 `AGENTS.md` / `CLAUDE.md` 区块与 `.ebo/WORKFLOW.md` 横幅，使 Agent 指令与模式一致。

提交纪律跟随模式。`strict` 保留溯源门：新建计划拒绝工作区中已有的源码改动，staged guard 要求已暂存源码由已完成计划与 receipt 背书。`silent` 放宽两者，使 vibe coding 可以在多个 Prompt 之间不提交地连续运行；Git 基线仍然必需，`guard check --staged` 以 `silent_mode_no_provenance` 放行。在静默模式下，Agent 只在用户要求时提交，且提交发生在点与点之间（`report` 之后、执行门关闭时），绝不中途提交；用户要求的提交是一次普通的 `git add -A && git commit`，累积的 `.ebo/` 记录随提交一并保存。`ebo status` 会打印 `uncommitted: N source change(s)` 提示，让用户决定何时"说提交"。

## 当前范围

- `.ebo/` 项目布局与根 Prompt 初始化。
- 生成中文 `.ebo/WORKFLOW.md`，内含每个常见 Ebo 流程的可复制提示词。
- `AGENTS.md` 与 `CLAUDE.md` 中的受管 Ebo 区块。
- Markdown Prompt 解析，支持 YAML Front Matter 子集。
- 从 stdin、文件或目录创建 Proposal。
- 哈希绑定的交互式审批。
- 通过校验过的临时候选树应用（apply）。
- 单根树校验、父节点检查、链接目标检查与 `depends_on` 环检查。
- 稳定的内容哈希与有效哈希。
- 确定性的脏节点扫描。
- `.ebo/plans/` 下的持久化执行计划。
- 基于计划的 `next`、`export`、`report`、`verify` 与 `abort`。
- `report passed|failed|blocked` 在计划哈希仍匹配时把执行状态写回 `.ebo/tree/`。
- 项目级锁文件 `.ebo/locks/project.lock`，用于变更类命令。
- fail-closed 执行门，配一个被忽略的 `.ebo/runtime/active-task.json` 租约。
- 计划与执行前的 Git 基线强制。
- `ebo guard check` 用于工作区授权与 staged 计划校验。
- 严格的 Agent 写入前决策，带确定性退出码与可选 JSON 输出。
- 项目级 Codex `PreToolUse` 适配器，通过 `ebo hooks install codex` 显式安装到 `.codex/hooks.json`。
- 可选的 Prompt `scope.allow` / `scope.deny` 通配符，用于文件级写授权。
- 可选的受管 pre-commit 钩子，通过 `ebo hooks install` 安装。
- 面向已完成计划的保守 `commit` 编排。
- 面向反向导入流程的基础证据包导出。
- `npm/` 下的 npm 根启动器与平台包骨架。
- 初始 npm 分发目标为 Windows x64 与 macOS arm64。

安装 npm 包只会把 `ebo` 可执行文件放到 `PATH` 上。它不会修改用户级 Codex 配置、项目 `.codex/` 文件或 Git 钩子。这些集成都是可选命令，必须在已初始化的 Ebo 项目内运行。

## 尚未完成

- 完整的 Cobra 命令层。
- 完整的 YAML 1.2 解析器集成与 JSON Schema 校验。
- 每个计划一个提交的实现代码自动暂存。
- Agent 原生写入前适配器的自动安装；Agent 运行时目前可以直接调用 `ebo hook pre-write`。
- 发布工作流发布 npm 包前，需要在 npm 上配置 Trusted Publishing。

## 发布

打 `vX.Y.Z` 标签会触发 `.github/workflows/release.yml`。

发布流程：

```text
go test ./...
构建 windows/amd64 与 darwin/arm64 二进制
写 dist/checksums.txt
node scripts/prepare-npm-packages.mjs X.Y.Z dist
对每个包执行 npm pack --dry-run
先发布平台包
最后发布 @aibo666/ebo
创建附带二进制与校验和的 GitHub Release
```

首个 npm 发布版本只包含：

```text
@aibo666/ebo
@aibo666/ebo-win32-x64
@aibo666/ebo-darwin-arm64
```

有需求时再添加其他平台。

## Prompt 边界

Ebo 只通过显式命令接受 Prompt 内容：

```bash
ebo add --stdin
ebo add --file drafts/feature.md
ebo add --dir drafts/prompts
```

这些命令只创建 Proposal。它们永远不会直接写入 `.ebo/tree/`。
