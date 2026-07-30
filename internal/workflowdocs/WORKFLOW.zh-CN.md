<!-- EBO:WORKFLOW:START -->
# Ebo 项目使用说明

> 此区块由 Ebo 管理。文档版本：`ebo.workflow/v1`。
> 可以在受管区块之外补充项目规则；再次运行 `ebo init` 时，区块之外的内容会被保留。

## 1. Ebo 的职责边界

- `.ebo/tree/` 是项目唯一、受 Git 版本控制的 Prompt 树，不在业务代码目录散放正式 Prompt。
- Ebo 不调用 AI，也不生成或执行代码；Agent 负责生成 Prompt 和实现代码，Ebo 负责收录、审批、计算状态和调度。
- Agent 不得直接修改 `.ebo/tree/` 中的 Prompt、状态或哈希。
- 新建或修改 Prompt 必须先成为 proposal，并经过人工 `review`、`approve` 和 `apply`。
- Agent 不得运行 `ebo approve` 或 `ebo apply`。

## 2. Prompt 标记与执行资格

每个 Prompt Markdown 都必须在 Front Matter 中包含状态标记：

```yaml
state:
  spec: approved
  execution: not_started
  sync: dirty
hash:
  satisfied: ""
```

| 标记或条件 | Ebo 与 Agent 的行为 |
| --- | --- |
| `project.root` | 仅作为树根，忽略，不执行 |
| `state.spec != approved` | 未通过人工审批，忽略 |
| `hash.satisfied` 等于当前 effective hash | 当前内容及依赖已经满足，忽略 |
| 内容哈希或依赖哈希发生变化 | 标记为需要重新执行 |
| `execution: failed` 或 `blocked` | 依赖就绪时允许重试 |
| 存在尚未满足且未审批的依赖，或其他未就绪依赖 | 当前 Prompt 暂不执行 |
| `ebo next` 没有返回任务 | 立即停止，不读取其他 Prompt 文件 |

执行资格由 Ebo 计算。Agent 不得通过遍历 `.ebo/tree/` 自行判断任务，也不得预加载无关分支。任何时候都只执行 `ebo next` 返回的一个任务。

## 3. 初始化后开始使用

用户先在项目根目录运行：

```bash
ebo init --agents codex,claude
```

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
只执行 ebo next 返回的单个任务，不要遍历或加载整个 Prompt 树。
选中任务后运行 ebo context <prompt-id> --depth 0，只读取当前 Prompt 和直接语义链接。
完成后使用任务包给出的 ebo report 命令报告结果，并运行 ebo verify <plan-id>。
```

Agent 应依次运行：

```bash
ebo status
ebo scan
ebo next
ebo context <prompt-id> --depth 0
```

## 5. 生成新的 Prompt

输入给 Agent：

```text
请根据下面的需求生成一个 Ebo Prompt Markdown。

生成前查询现有 Prompt 树，确定唯一 parent，并分析 depends_on、affects、implements、references 和 supersedes 关系；语义链接需要说明 reason。
把生成文件写到 .ebo/tree 之外，例如 drafts/<name>.md。
不要运行 ebo approve 或 ebo apply；除非我明确要求，也不要运行 ebo add。

需求：
<在这里填写需求>
```

## 6. 修改已有 Prompt

输入给 Agent：

```text
请为 Ebo Prompt <prompt-id> 生成修改版本。

先读取该 Prompt 和直接语义链接，分析内容变化会影响哪些依赖节点。
把修改后的完整 Markdown 写到 .ebo/tree 之外，例如 drafts/<prompt-id>.md。
不要直接修改原 Prompt，不要运行 ebo approve 或 ebo apply。

修改要求：
<在这里填写修改内容>
```

## 7. 人工审阅并加入 Prompt 树

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

`ebo approve` 会要求用户在交互终端输入完整 proposal hash。只有通过这一步，Prompt 才能成为可执行工作。

## 8. 继续已有执行计划

输入给 Agent：

```text
请继续当前 Ebo 执行计划。

运行 ebo status、ebo plan list 和 ebo next。
只处理 ebo next 返回的任务；如果没有任务，立即停止并告诉我当前计划状态。
不要预加载其他 Prompt。
```

## 9. 处理失败或阻塞

输入给 Agent：

```text
请检查当前 Ebo 计划中 failed 或 blocked 的下一个任务。

只读取 ebo next 返回的任务、执行报告和直接依赖，说明失败或阻塞原因。
如果能够修复，只修复这个任务；在得到我的确认前，不要修改 Prompt 规格，也不要伪造 passed 状态。
完成后如实运行 ebo report，并运行 ebo verify <plan-id>。
```

## 10. 从已有项目反向抽象 Prompt 树

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

## 11. 完成任务并验证

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

确认业务代码已经正确暂存后，再运行：

```bash
ebo commit <plan-id> --message "<提交说明>"
```
<!-- EBO:WORKFLOW:END -->
