# ADMIN_PAYMENT_INTEGRATION_API

> 单文件中英双语文档 / Single-file bilingual documentation (Chinese + English)

---

## 中文

### 目标
本文档用于对接外部支付系统（如 `sub2apipay`）与 Sub2API 的 Admin API，覆盖：
- 支付成功后充值
- 用户查询
- 人工余额修正
- 前端购买页参数透传

### 基础地址
- 生产：`https://<your-domain>`
- Beta：`http://<your-server-ip>:8084`

### 认证
推荐使用：
- `x-api-key: admin-<64hex>`
- `Content-Type: application/json`
- 幂等接口额外传：`Idempotency-Key`

说明：管理员 JWT 也可访问 admin 路由，但服务间调用建议使用 Admin API Key。

### 1) 一步完成创建并兑换
`POST /api/v1/admin/redeem-codes/create-and-redeem`

用途：原子完成“创建兑换码 + 兑换到指定用户”。

请求头：
- `x-api-key`
- `Idempotency-Key`

请求体示例：
```json
{
  "code": "s2p_cm1234567890",
  "type": "balance",
  "value": 100.0,
  "user_id": 123,
  "notes": "sub2apipay order: cm1234567890"
}
```

幂等语义：
- 同 `code` 且 `used_by` 一致：`200`
- 同 `code` 但 `used_by` 不一致：`409`
- 缺少 `Idempotency-Key`：`400`（`IDEMPOTENCY_KEY_REQUIRED`）

curl 示例：
```bash
curl -X POST "${BASE}/api/v1/admin/redeem-codes/create-and-redeem" \
  -H "x-api-key: ${KEY}" \
  -H "Idempotency-Key: pay-cm1234567890-success" \
  -H "Content-Type: application/json" \
  -d '{
    "code":"s2p_cm1234567890",
    "type":"balance",
    "value":100.00,
    "user_id":123,
    "notes":"sub2apipay order: cm1234567890"
  }'
```

### 2) 查询用户（可选前置校验）
`GET /api/v1/admin/users/:id`

```bash
curl -s "${BASE}/api/v1/admin/users/123" \
  -H "x-api-key: ${KEY}"
```

### 3) 余额调整（已有接口）
`POST /api/v1/admin/users/:id/balance`

用途：人工补偿 / 扣减，支持 `set` / `add` / `subtract`。

请求体示例（扣减）：
```json
{
  "balance": 100.0,
  "operation": "subtract",
  "notes": "manual correction"
}
```

```bash
curl -X POST "${BASE}/api/v1/admin/users/123/balance" \
  -H "x-api-key: ${KEY}" \
  -H "Idempotency-Key: balance-subtract-cm1234567890" \
  -H "Content-Type: application/json" \
  -d '{
    "balance":100.00,
    "operation":"subtract",
    "notes":"manual correction"
  }'
```

### 4) 外部小铺购买兑换码
管理员在支付设置中为 10 / 20 / 30 / 50 / 100 元五个固定档位分别填写商品链接，并启用 `purchase_subscription_enabled`。外部购买独立于 `payment_enabled`，并优先于内置支付。每档使用自己的链接，没有自定义金额；商品价格和兑换码面额以对应小铺商品为准。

配置字段 `purchase_subscription_products` 是 JSON 对象，键只能是字符串 `"10"`、`"20"`、`"30"`、`"50"`、`"100"`，值为该档位的完整 http(s) 商品 URL。后台提供五行独立输入及安全预览。链接不得包含用户名、密码、控制字符或反斜杠，长度不超过 2048 字节，主机和端口必须合法。空值视为未配置；禁用时可以保存空配置，启用时至少需要一个有效商品链接。部分更新省略此字段会保留现值，发送 `{}` 会清空配置。旧 `purchase_subscription_url` 仅保留兼容读取和存储，不作为档位链接的回退，也不会自动迁移。

未配置档位不可选择。用户选择可用商品并点击“前往购买”后，新标签页原样打开该档位的链接，不附加金额、身份或会话参数。用户在小铺购买兑换码后返回 `/redeem` 兑换。无可用商品时提示联系管理员，已有码仍可兑换。

### 5) 失败处理建议
- 支付成功与充值成功分状态落库
- 回调验签成功后立即标记“支付成功”
- 支付成功但充值失败的订单允许后续重试
- 重试保持相同 `code`，并使用新的 `Idempotency-Key`

### 6) `doc_url` 配置建议
- 查看链接：`https://github.com/loLollipop/Sub2api/blob/main/docs/ADMIN_PAYMENT_INTEGRATION_API.md`
- 下载链接：`https://raw.githubusercontent.com/loLollipop/Sub2api/main/docs/ADMIN_PAYMENT_INTEGRATION_API.md`

---

## English

### Purpose
This document describes the minimal Sub2API Admin API surface for external payment integrations (for example, `sub2apipay`), including:
- Recharge after payment success
- User lookup
- Manual balance correction
- Purchase page query parameter forwarding

### Base URL
- Production: `https://<your-domain>`
- Beta: `http://<your-server-ip>:8084`

### Authentication
Recommended headers:
- `x-api-key: admin-<64hex>`
- `Content-Type: application/json`
- `Idempotency-Key` for idempotent endpoints

Note: Admin JWT can also access admin routes, but Admin API Key is recommended for server-to-server integration.

### 1) Create and Redeem in one step
`POST /api/v1/admin/redeem-codes/create-and-redeem`

Use case: atomically create a redeem code and redeem it to a target user.

Headers:
- `x-api-key`
- `Idempotency-Key`

Request body:
```json
{
  "code": "s2p_cm1234567890",
  "type": "balance",
  "value": 100.0,
  "user_id": 123,
  "notes": "sub2apipay order: cm1234567890"
}
```

Idempotency behavior:
- Same `code` and same `used_by`: `200`
- Same `code` but different `used_by`: `409`
- Missing `Idempotency-Key`: `400` (`IDEMPOTENCY_KEY_REQUIRED`)

curl example:
```bash
curl -X POST "${BASE}/api/v1/admin/redeem-codes/create-and-redeem" \
  -H "x-api-key: ${KEY}" \
  -H "Idempotency-Key: pay-cm1234567890-success" \
  -H "Content-Type: application/json" \
  -d '{
    "code":"s2p_cm1234567890",
    "type":"balance",
    "value":100.00,
    "user_id":123,
    "notes":"sub2apipay order: cm1234567890"
  }'
```

### 2) Query User (optional pre-check)
`GET /api/v1/admin/users/:id`

```bash
curl -s "${BASE}/api/v1/admin/users/123" \
  -H "x-api-key: ${KEY}"
```

### 3) Balance Adjustment (existing API)
`POST /api/v1/admin/users/:id/balance`

Use case: manual correction with `set` / `add` / `subtract`.

Request body example (`subtract`):
```json
{
  "balance": 100.0,
  "operation": "subtract",
  "notes": "manual correction"
}
```

```bash
curl -X POST "${BASE}/api/v1/admin/users/123/balance" \
  -H "x-api-key: ${KEY}" \
  -H "Idempotency-Key: balance-subtract-cm1234567890" \
  -H "Content-Type: application/json" \
  -d '{
    "balance":100.00,
    "operation":"subtract",
    "notes":"manual correction"
  }'
```

### 4) External shop redeem codes
In payment settings, configure a separate product URL for each fixed CNY 10 / 20 / 30 / 50 / 100 tier and enable `purchase_subscription_enabled`. External purchases work independently of `payment_enabled` and take precedence over built-in payments. Each tier uses its own URL, with no custom amount; prices and code values are determined by the corresponding shop product.

`purchase_subscription_products` is a JSON object with only the string keys `"10"`, `"20"`, `"30"`, `"50"`, and `"100"`. Values are absolute http(s) URLs for the corresponding products. The admin page provides five independent inputs with safe previews. URLs must have valid hosts and ports, no credentials, control characters or backslashes, and at most 2048 bytes. Blank values mean unconfigured. Disabled purchases may have an empty configuration; enabling requires at least one valid product URL. Omitting the field in a partial update preserves it; sending `{}` clears it. The legacy `purchase_subscription_url` remains readable and stored for compatibility, without tier fallback or automatic migration.

Unconfigured tiers cannot be selected. Selecting an available product and clicking “Go to shop” opens that tier’s exact URL in a new tab without adding amount, identity or session parameters. Users buy a code in the shop and return to `/redeem`. When no products are available, users are directed to contact an administrator; existing codes can still be redeemed.

### 5) Failure handling recommendations
- Persist payment success and recharge success as separate states
- Mark payment as successful immediately after verified callback
- Allow retry for orders with payment success but recharge failure
- Keep the same `code` for retry, and use a new `Idempotency-Key`

### 6) Recommended `doc_url`
- View URL: `https://github.com/loLollipop/Sub2api/blob/main/docs/ADMIN_PAYMENT_INTEGRATION_API.md`
- Download URL: `https://raw.githubusercontent.com/loLollipop/Sub2api/main/docs/ADMIN_PAYMENT_INTEGRATION_API.md`
