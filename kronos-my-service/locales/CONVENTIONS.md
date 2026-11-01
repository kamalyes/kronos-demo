# 本地化文件 (Locales)

本目录包含系统支持的多语言错误消息翻译文件

## 支持的语言

| 文件名 | 语言 | 区域 |
|--------|------|------|
| `en.json` | English | 英语 |
| `zh.json` | 简体中文 | 中国大陆 |
| `zh-tw.json` | 繁體中文（繁體新碼） | 台湾 |
| `tc.json` | 繁體中文 | 台湾/香港 |
| `ja.json` | 日本語 (Japanese) | 日语 |
| `ko.json` | 한국어 (Korean) | 韩语 |
| `fr.json` | Français (French) | 法语 |
| `fr-fr.json` | Français (France) | 法语（法国） |
| `de.json` | Deutsch (German) | 德语 |
| `es.json` | Español (Spanish) | 西班牙语 |
| `it.json` | Italiano (Italian) | 意大利语 |
| `pt.json` | Português (Portuguese) | 葡萄牙语 |
| `pt-br.json` | Português (Brasil) | 葡萄牙语（巴西） |
| `nl.json` | Nederlands (Dutch) | 荷兰语 |
| `sv.json` | Svenska (Swedish) | 瑞典语 |
| `ru.json` | Русский (Russian) | 俄语 |
| `tr.json` | Türkçe (Turkish) | 土耳其语 |
| `ar.json` | العربية (Arabic) | 阿拉伯语 |
| `hi.json` | हिन्दी (Hindi) | 印地语 |
| `bn.json` | বাংলা (Bengali) | 孟加拉语 |
| `bm.json` | Bahasa Melayu | 马来语 |
| `my.json` | မြန်မာဘာသာ (Burmese) | 缅甸语 |
| `id.json` | Bahasa Indonesia | 印尼语 |
| `kh.json` | ភាសាខ្មែរ (Khmer) | 高棉语/柬埔寨语 |
| `lo.json` | ພາສາລາວ (Lao) | 老挝语 |
| `th.json` | ภาษาไทย (Thai) | 泰语 |
| `ur.json` | اردو (Urdu) | 乌尔都语 |
| `vi.json` | Tiếng Việt (Vietnamese) | 越南语 |

## 文件结构

每个语言文件都遵循相同的JSON结构：开头是几个通用占位键，错误文案全部放在 `error` 对象下

```json
{
  "welcome": "Welcome",
  "success": "Success",
  "hello": "Hello",
  "goodbye": "Goodbye",
  "error": {
    "product_not_found": "Product not found",
    "product_already_exists": "Product already exists: {{reason}}"
  }
}
```

go-i18n 在加载时把嵌套结构按点号拍平（见 `FlattenToMessages`），所以 `error` 对象里的
`product_not_found` 查找键就是 `error.product_not_found`——这正好是 `errcodes` 里那个码的值

**本仓库是 kopy 模板**，所以实体名是占位符，`error` 整块也包在
` ... ` 里：关掉示例时只保留前 4 个占位键，
JSON 两种情况下都合法改这些文件时不要把条件块拆开，否则关掉示例那条路径会渲染出
多一个逗号的非法 JSON

通用错误码（`error.not_found`、`error.conflict` 等）**不在这里**：它们的文案由 go-bizerror
内置兜底，服务侧只在需要覆盖时才写同名键

## 添加新语言

1. 复制 `en.json` 作为模板
2. 重命名为对应的语言代码（如 `fr.json` 表示法语）
3. 翻译所有错误消息文本
4. 更新本README文档中的支持语言列表
5. 确保JSON格式正确（可使用在线JSON验证工具）

## 添加新的错误消息键

在所有语言文件中添加相同的键，确保多语言支持的一致性：

1. 在 `en.json` 中添加新的错误消息键和英文文本
2. 在所有其他语言文件中添加相同的键和对应的翻译
3. 保持所有文件的键顺序一致，便于维护

## 使用说明

系统会根据请求的语言参数（如HTTP Header中的 `Accept-Language`）自动加载对应的语言文件如果请求的语言不存在，将回退到默认语言（英语）

### 在代码中使用 i18n

#### 1. 导入必要的包

```go
import (
    gatewayMiddleware "github.com/kamalyes/go-rpc-gateway/middleware"
    bizerror "github.com/kamalyes/go-bizerror"
    "github.com/kamalyes/kronos-my-service/errcodes"
)
```

通用码在 `bizerror`（go-bizerror），实体专属码在本服务的 `errcodes` 包——不要再找
`<项目>/errors`，那个包不存在，错误构造函数在 go-bizerror 里

#### 2. 简单消息翻译

使用 `T` 函数获取翻译文本：

```go
// 获取简单的错误消息
message := gatewayMiddleware.T(ctx, errcodes.BizErrCodeProductNotFound)
```

#### 3. 带变量的消息翻译

使用 `TWithMap` 函数支持模板变量：

```go
// 带变量的消息（支持 {{variableName}} 与 {{.variableName}} 两种模板写法）
message := gatewayMiddleware.TWithMap(ctx, "error.agent_assigned_success", map[string]any{
    "agentId": agentID,
})
```

#### 4. 在错误返回中使用

```go
// 返回带状态码的业务错误：文案由 go-bizerror 按请求语言解析，不要自己拼句子
if model == nil {
    return nil, bizerror.ToGRPCError(ctx, errcodes.BizErrCodeProductNotFound)
}

// 带模板变量的
if exists {
    return nil, bizerror.ToGRPCErrorWithTemplate(ctx, errcodes.BizErrCodeProductAlreadyExists,
        map[string]any{"reason": req.GetCode()})
}
```

⚠️ 带占位符的文案一定要走带模板的函数`GetMsgByKey` / `T` 不套模板，调用方会收到字面量
`{{reason}}`需要直接取文案（比如批量接口的逐条 `error_message`）时用
`gatewayMiddleware.GetMsgWithMap(ctx, code, map[string]any{"reason": email})`

### 模板变量语法

在 JSON 文件中使用双花括号定义变量：

- `{{variableName}}`
- `{{.variableName}}`

建议业务侧优先统一使用 `{{variableName}}`，更简洁；历史文案中的 `{{.variableName}}` 也完全兼容

```json
{
  "error": {
    "agent_assigned_success": "已分配给客服 {{agentId}}",
    "user_not_found": "用户 {{.username}} 不存在"
  }
}
```

在代码中传入对应的变量：

```go
gatewayMiddleware.TWithMap(ctx, "error.agent_assigned_success", map[string]any{
    "agentId": "agent123",
})
// 输出：已分配给客服 agent123

gatewayMiddleware.TWithMap(ctx, "error.user_not_found", map[string]any{
    "username": "john",
})
// 输出：用户 john 不存在
```

### 最佳实践

1. **统一命名空间**：所有错误消息使用 `error.*` 命名空间
2. **复用优先**：优先使用已有的 i18n key，避免重复定义
3. **语义化命名**：key 名称要清晰表达含义，如 `ticket_not_found` 而不是 `err1`
4. **完整覆盖**：新增 key 必须在所有语言文件中同步添加（当前共 28 个），且各文件的键集合
   完全相等——少一个文件就只有那种语言会回退，而这种 bug 只在特定 `Accept-Language` 下出现
5. **码与键同源**：错误码的值就是它的 i18n key，新增码时 `errcodes/codes.go` 的常量、
   `errcodes.Mappings()` 的映射、28 个语言文件的键三件事要一起做；只做前一件的话该码会
   以 500 返回（除非码值以 `_not_found` 结尾）
6. **模板变量**：使用有意义的变量名，如 `agentId` 而不是 `id`

### 常见问题

**Q: 如何处理 `fmt.Errorf` 的 linter 警告？**

A: 使用 `errors.New()` 替代：

```go
// ❌ 会触发 linter 警告
return fmt.Errorf(gatewayMiddleware.T(ctx, "error.xxx"))

// ✅ 正确做法
return errors.New(gatewayMiddleware.T(ctx, "error.xxx"))
```

**Q: 模板变量不生效怎么办？**

A: 检查以下几点：

- 使用 `TWithMap` 而不是 `T`
- 变量名使用 `{{variableName}}` 或 `{{.variableName}}`
- map 的 key 与模板变量名一致

**Q: 如何查看已有的 i18n key？**

A: 本服务自己的 key 看 `locales/en.json` 或 `locales/zh.json`，错误类的 key 都在 `error`
对象下；跨服务通用码的兜底文案不在这里，见 go-bizerror 的 `builtin_messages.go`

## 翻译注意事项

1. **保持专业术语一致性**：工单(Ticket)、客服(Agent)、会话(Session)等术语在同一语言中应保持一致
2. **尊重语言习惯**：不要直接机翻，应符合目标语言的表达习惯
3. **占位符处理**：如果未来添加带参数的错误消息（如 `{{username}} not found` 或 `{{.username}} not found`），确保占位符在翻译后保持不变
4. **长度考虑**：某些语言的翻译可能比英语长，前端UI应考虑文本长度的适配
5. **文化敏感性**：避免可能在特定文化中不恰当的表达

## 测试

添加或修改翻译后，应进行以下测试：

1. JSON格式验证（确保没有语法错误）
2. 键完整性检查（确保所有语言文件包含相同的键）
3. 实际场景测试（在不同语言环境下触发错误消息）

## 维护

- **定期审查**：随着产品迭代，定期审查翻译的准确性和时效性
- **版本控制**：所有翻译变更都应通过Git进行版本控制
- **协作翻译**：建议由母语者或专业译者审核翻译质量
