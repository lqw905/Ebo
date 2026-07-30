<!-- EBO:WORKFLOW:START -->
# Ebo 项目使用说明

> 此区块由 Ebo 管理。文档版本：`ebo.workflow/v1`。
> 可以在受管区块之外补充项目规则；再次运行 `ebo init` 时，区块之外的内容会被保留。

## 0. 最高优先级执行门禁

```text
EBO HARD GATE: NO EBO TASK, NO SOURCE EDIT.
没有 Ebo 任务，就不能修改业务代码。
```

- `ebo scan` 只显示候选任务，永远不授予代码修改权限。
- 只有 `ebo next` 输出 `EBO EXECUTION GATE: OPEN` 时，Agent 才能修改业务代码。
- `.ebo/runtime/active-task.json` 记录当前唯一活动任务；`ebo report` 完成后门禁立即关闭。
- `ebo next` 输出 `CLOSED` 或没有任务时，Agent 必须停止业务代码修改。
- 此时如果用户提出代码变更，视为授权 Agent 生成草稿并运行 `ebo add` 创建 proposal，但不代表授权 `approve` 或 `apply`。
- proposal 创建后 Agent 必须停止，等用户人工审批和应用；再次获得 `OPEN` 后才能实现。
- 每次调用 Write、Edit、Patch 或其他文件写入工具前运行 `ebo hook pre-write --path <目标文件>`；只有退出码为 `0` 且返回 `EBO PRE-WRITE: ALLOWED` 才能写入。
- `drafts/**/*.md` 是唯一不需要 OPEN active task 的 Agent 写入区域，仅用于 proposal 草稿，并不授予业务源码修改权限。

## 1. Ebo 的职责边界

- `.ebo/tree/` 是项目唯一、受 Git 版本控制的 Prompt 树，不在业务代码目录散放正式 Prompt。
- Ebo 不调用 AI，也不生成或执行代码；Agent 负责生成 Prompt 和实现代码，Ebo 负责收录、审批、计算状态和调度。
- Agent 不得直接修改 `.ebo/tree/` 中的 Prompt、状态或哈希。
- Agent 不得手工伪造或修改 `.ebo/plans/`、`.ebo/receipts/` 和 `.ebo/runtime/active-task.json`；这些控制状态只能由 Ebo 命令写入。
- 新建或修改 Prompt 必须先成为 proposal，并经过人工 `review`、`approve` 和 `apply`。
- Agent 不得运行 `ebo approve` 或 `ebo apply`。
- Prompt 树执行依赖 Git 基线；没有至少一个 Git commit 时，`ebo plan` 和 `ebo next` 会拒绝执行。
- 全新 plan 第一次执行前必须没有既存业务代码改动；否则 `ebo next` 会以 `preexisting_source_changes` 关闭门禁，防止先改代码再补任务。

## 2. Prompt 标记与执行资格

每个 Prompt Markdown 都必须在 Front Matter 中包含状态标记：

```yaml
state:
  spec: approved
  execution: not_started
  sync: dirty
hash:
  satisfied: ""
scope:
  allow:
    - internal/auth/**
  deny:
    - internal/auth/secrets/**
```

`scope` 可选。不填写 `allow` 时，活动任务可以修改除 Ebo、Git 和 Agent 控制文件外的业务文件；填写后，pre-write Hook 只放行匹配路径。`deny` 始终优先，支持 `*`、`?` 和跨目录的 `**`。

| 标记或条件 | Ebo 与 Agent 的行为 |
| --- | --- |
| `project.root` | 仅作为树根，忽略，不执行 |
| `state.spec != approved` | 未通过人工审批，忽略 |
| `hash.satisfied` 等于当前 effective hash | 当前内容及依赖已经满足，忽略 |
| 内容哈希或依赖哈希发生变化 | 标记为需要重新执行 |
| `execution: failed` 或 `blocked` | 依赖就绪时允许重试 |
| 存在尚未满足且未审批的依赖，或其他未就绪依赖 | 当前 Prompt 暂不执行 |
| `ebo scan` 返回任务 | 仅供查看，门禁仍为 CLOSED |
| `ebo next` 返回 OPEN | 获得一个活动任务，可以修改业务代码 |
| `ebo next` 返回 CLOSED | 立即停止，不读取或修改业务代码 |
| 目标文件不匹配 `scope.allow` 或命中 `scope.deny` | pre-write Hook 拒绝写入 |

执行资格由 Ebo 计算。Agent 不得通过遍历 `.ebo/tree/` 自行判断任务，也不得预加载无关分支。任何时候都只执行 `ebo next` 返回的一个任务。

## 3. 初始化后开始使用

用户先在项目根目录运行：

```bash
ebo init --agents codex,claude
```

检查初始化文件并创建 Git 基线；Ebo 不会代替用户提交业务文件：

```bash
git status
git add .ebo .gitignore AGENTS.md CLAUDE.md
git commit -m "chore: initialize ebo"
ebo doctor
```

需要阻止绕过 Ebo 的 Git 提交时，可选择安装受管 pre-commit hook：

```bash
ebo hooks install
ebo hooks status
```

Agent 平台的写文件前 Hook 应调用：

```bash
ebo hook pre-write --path <目标文件>
```

该命令不调用 AI、不访问网络。退出码 `0` 表示本次写入允许，`1` 表示门禁拒绝，`2` 表示 Ebo 配置或运行状态损坏。Git pre-commit Hook 与 Agent pre-write Hook 相互独立：前者保护提交，后者保护工作区。

Codex 可以在当前已初始化的 Ebo 项目中安装项目级 `PreToolUse` 适配器：

```bash
ebo hooks install codex
ebo hooks status codex
```

该命令只会合并写入当前项目的 `.codex/hooks.json`，不会修改用户目录下的 `~/.codex/hooks.json`。安装后在 Codex 中打开 `/hooks`，审阅并信任新增定义。该 Hook 只拦截 `Edit`、`Write` 和 `apply_patch`；Shell 写文件仍需遵守本工作流，并由 Git Hook 和 CI 兜底。

安装 npm 包本身不会修改任何 Codex 配置或 Git Hook。只有用户在已初始化项目中显式运行上述安装命令，Ebo 才会写入项目级配置。

然后输入给 Agent：

```text
请先完整阅读 .ebo/WORKFLOW.md，并按照其中的 Ebo 工作流操作这个项目。
不要直接修改 .ebo/tree 中的 Prompt、状态或哈希，也不要执行人工审批命令。
```

## 4. 执行下一个 Prompt

输入给 Agent：

```text
请根据 Ebo 执行当前项目的下一个任务。

先运行 ebo status、ebo scan 和 ebo next。
注意 ebo scan 不授予执行权限。只有 ebo next 返回 EBO EXECUTION GATE: OPEN 后，才能执行它返回的单个任务。
随后针对每个准备写入的文件运行 ebo hook pre-write --path <目标文件>；结果不是 ALLOWED 时立即停止。
选中任务后运行 ebo context <prompt-id> --depth 0，只读取当前 Prompt 和直接语义链接。
完成后使用任务包给出的 ebo report 命令报告结果，并运行 ebo verify <plan-id>。
```

Agent 应依次运行：

```bash
ebo status
ebo scan
ebo next
ebo hook pre-write --path <目标文件>
ebo context <prompt-id> --depth 0
```

## 5. 将对话沉淀为 Prompt 树

用户可以先与 Agent 自由讨论需求、方案和边界。讨论内容不会自动进入 Prompt 树。当用户明确要求“沉淀到 Ebo”“更新 Prompt 树”或直接提出新的代码变更，而当前没有 OPEN 任务时，Agent 可以创建草稿并运行 `ebo add`，但不能修改业务代码。

讨论完成后，输入给 Agent：

```text
请把刚才的讨论沉淀到 Ebo Prompt 树。

分析哪些内容应该新建 Prompt、修改已有 Prompt 或通过 supersedes 替代旧 Prompt，补全目标、上下文、验收条件、唯一 parent 和语义依赖。
先查询相关 Prompt，只加载必要上下文。把本次讨论形成的完整 Markdown 写到独立目录 drafts/ebo/<topic>/。
你可以先运行 ebo add --dry-run --dir drafts/ebo/<topic>/ 检查，再运行 ebo add --dir drafts/ebo/<topic>/ 创建一个 proposal。
创建完成后告诉我 proposal ID、proposal hash、新增或修改的 Prompt、依赖变化和待确认问题。
不要直接修改 .ebo/tree，不要运行 ebo approve 或 ebo apply。
```

Agent 应按需查询相关节点，并把同一次讨论产生的相关 Prompt 放入一个原子 proposal：

```bash
ebo tree search "<相关内容>"
ebo context <相关-prompt-id> --depth 0
ebo add --dry-run --dir drafts/ebo/<topic>/
ebo add --dir drafts/ebo/<topic>/
```

如果本次讨论只产生一个 Prompt，也可以使用 `ebo add --file <path>`。Agent 创建 proposal 后必须停止；真正更新 `.ebo/tree/` 仍由用户执行 `review`、`approve` 和 `apply`。

Agent 应向用户返回类似摘要：

```text
Proposal ID: <proposal-id>
Proposal Hash: <proposal-hash>

新增 Prompt：
- <prompt-id>

修改 Prompt：
- <prompt-id>

依赖变化：
- <prompt-id> depends_on <prompt-id>

待确认问题：
- <问题>

请人工运行：
ebo review <proposal-id>
ebo approve <proposal-id>
ebo apply <proposal-id>
```

## 6. 生成新的 Prompt

输入给 Agent：

```text
请根据下面的需求生成一个 Ebo Prompt Markdown。

生成前查询现有 Prompt 树，确定唯一 parent，并分析 depends_on、affects、implements、references 和 supersedes 关系；语义链接需要说明 reason。
把生成文件写到 .ebo/tree 之外，例如 drafts/<name>.md。
不要运行 ebo approve 或 ebo apply；除非我明确要求，也不要运行 ebo add。

需求：
<在这里填写需求>
```

## 7. 修改已有 Prompt

输入给 Agent：

```text
请为 Ebo Prompt <prompt-id> 生成修改版本。

先读取该 Prompt 和直接语义链接，分析内容变化会影响哪些依赖节点。
把修改后的完整 Markdown 写到 .ebo/tree 之外，例如 drafts/<prompt-id>.md。
不要直接修改原 Prompt，不要运行 ebo approve 或 ebo apply。

修改要求：
<在这里填写修改内容>
```

## 8. 人工审阅并加入 Prompt 树

用户先把草稿登记为 proposal：

```bash
ebo add --file drafts/<name>.md
ebo review <proposal-id>
```

命令输出 proposal ID 后，可以让 Agent 帮助审阅，但不能让它审批。输入给 Agent：

```text
请运行 ebo review <proposal-id>，只审阅这个 Ebo proposal。
检查目标、验收条件、父子关系和依赖关系是否完整，并列出风险。
不要运行 ebo approve 或 ebo apply。
```

确认审阅结果后，用户亲自运行：

```bash
ebo approve <proposal-id>
ebo apply <proposal-id>
```

`ebo approve` 会显示 proposal 摘要和短哈希，并要求用户在交互终端输入 `y` 确认。Ebo 会在内部重新计算并保存完整 proposal hash；`ebo apply` 前还会再次校验，内容一旦变化就拒绝应用。只有通过这一步，Prompt 才能成为可执行工作。

## 9. 继续已有执行计划

输入给 Agent：

```text
请继续当前 Ebo 执行计划。

运行 ebo status、ebo plan list 和 ebo next。
只处理 ebo next 返回的任务；如果没有任务，立即停止并告诉我当前计划状态。
不要预加载其他 Prompt。
```

## 10. 处理失败或阻塞

输入给 Agent：

```text
请检查当前 Ebo 计划中 failed 或 blocked 的下一个任务。

只读取 ebo next 返回的任务、执行报告和直接依赖，说明失败或阻塞原因。
如果能够修复，只修复这个任务；在得到我的确认前，不要修改 Prompt 规格，也不要伪造 passed 状态。
完成后如实运行 ebo report，并运行 ebo verify <plan-id>。
```

## 11. 从已有项目反向抽象 Prompt 树

用户先生成项目证据：

```bash
ebo import . --out .ebo/runtime/import
```

然后输入给 Agent：

```text
请根据 .ebo/runtime/import 中的项目证据，把现有代码、架构和功能反向抽象成 Ebo Prompt 树草稿。

以 project.root 为唯一根节点，为架构和功能 Prompt 建立父子关系与语义依赖。
所有生成文件都写到 .ebo/tree 之外，例如 drafts/imported/。
不要自动运行 ebo approve 或 ebo apply，生成后交给我逐项审阅。
```

## 12. 完成任务并验证

输入给 Agent：

```text
请完成当前 ebo next 返回的任务，并根据实际结果运行任务包中的 ebo report 命令。
然后运行 ebo verify <plan-id>。
如果验证未完成或失败，不要提交，也不要把状态改成 passed；请说明剩余问题。
```

计划完成后，用户可以先预览 Ebo 将提交的控制文件：

```bash
ebo commit <plan-id> --dry-run
```

先暂存本计划产生的业务代码。`ebo commit` 会自动暂存对应的 Prompt 树、plan 和 receipt，并在真正提交前执行 staged guard：

```bash
git add <本计划修改的业务文件>
ebo commit <plan-id> --dry-run
```

确认后提交：

```bash
ebo commit <plan-id> --message "<提交说明>"
```

如果希望在提交前单独运行 staged guard，需要先把控制文件也暂存：

```bash
git add <本计划修改的业务文件> .ebo/tree .ebo/plans/<plan-id>.json .ebo/receipts
ebo guard check --staged
```

可选 pre-commit hook 会阻止直接运行 `git commit` 绕过这项检查；即使没有安装 hook，`ebo commit` 本身也会执行同等校验。
<!-- EBO:WORKFLOW:END -->
