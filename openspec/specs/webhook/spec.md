## Purpose

定义 StorageBackendClaim 准入验证 Webhook 规范，在创建和更新时验证 SBC 资源，确保满足架构和业务规则，包括 Provider 必填校验、ConfigMapMeta/SecretMeta 格式校验和不可变字段校验。

## Requirements

### Requirement: sbc-validation SHALL validate StorageBackendClaim on create and update
准入 Webhook SHALL 验证 StorageBackendClaim 资源，以确保它们在持久化之前满足所需的架构和业务规则。Webhook 路径为 "/storagebackendclaim"，失败策略为 "Fail"。不执行变更操作——Webhook 纯粹是验证性的。

#### Scenario:拒绝 Content-Type 非 application/json 的请求
- **WHEN** Webhook 收到 Content-Type 不为 "application/json" 的 HTTP 请求时
- **THEN** Webhook 拒绝请求并返回错误

#### Scenario:拒绝空请求体
- **WHEN** Webhook 收到 nil body 的 HTTP 请求时
- **THEN** Webhook 拒绝请求并返回错误

#### Scenario:拒绝 body 读取失败的请求
- **WHEN** Webhook 读取请求体失败时
- **THEN** Webhook 拒绝请求并返回 "Request could not be decoded: <error>" 错误

#### Scenario:拒绝不支持的 AdmissionReview GVK
- **WHEN** Webhook 解码请求后发现对象既非 v1.AdmissionReview 也非 v1beta1.AdmissionReview 时
- **THEN** Webhook 返回 "unsupported group version" 错误；注意：由于 scheme.go 仅注册了 admissionV1 而未注册 admissionV1beta1，若 API Server 发送 v1beta1 AdmissionReview，UniversalDeserializer 将无法解码，请求会在 Decode 阶段失败（返回 "Request could not be decoded" 错误），而非走到 "unsupported group version" 分支

#### Scenario:拒绝解码 newObject 失败的请求
- **WHEN** Webhook 解码 AdmissionReview 的 newObjectRaw 失败时
- **THEN** Webhook 返回解码错误

#### Scenario:拒绝解码 oldObject 失败的更新请求
- **WHEN** Webhook 在处理 Update 操作时解码 oldObjectRaw 失败时
- **THEN** Webhook 返回解码错误

#### Scenario:允许 Connect 操作
- **WHEN** Webhook 收到 admissionV1.Connect 操作时
- **THEN** Webhook 直接返回 nil（允许请求），不执行任何验证；注意：Connect 不在 AdmissionOps 列表中（仅注册 Create、Update、Delete），因此此处理器为死代码，Kubernetes API Server 不会将 Connect 操作路由到此 webhook

#### Scenario:未知操作被允许
- **WHEN** Webhook 收到未知操作（非 Create、Update、Delete、Connect）时
- **THEN** validateStorageBackendClaim 记录错误日志 "the operation [%s] to StorageBackendClaim [%s] is unknown, refuse default" 但仍返回 nil（允许请求）；日志中写 "refuse default" 但实际行为是允许

#### Scenario:验证 SBC ConfigMapMeta 不为空
- **WHEN** 用户创建 ConfigMapMeta 为空的 SBC 时
- **THEN** Webhook 拒绝请求，错误为 "StorageBackendClaim %s's configmap [%s] is empty"

#### Scenario:验证 SBC SecretMeta 不为空
- **WHEN** 用户创建 SecretMeta 为空的 SBC 时
- **THEN** Webhook 拒绝请求，错误为 "StorageBackendClaim %s's secret [%s] is empty"

#### Scenario:验证带有必需 Provider 字段的 SBC
- **WHEN** 用户创建不带 Provider 字段的 SBC 时
- **THEN** Webhook 拒绝请求，错误为 "Provider in StorageBackendClaim [%s] can not be empty"

#### Scenario:验证 SBC ConfigMapMeta 格式
- **WHEN** 用户创建 ConfigMapMeta 不符合 "<namespace>/<name>" 格式的 SBC 时
- **THEN** Webhook 拒绝请求；格式在获取后端信息期间间接验证，返回错误 "unexpected key format: <key>"

#### Scenario:验证 SBC SecretMeta 格式
- **WHEN** 用户创建 SecretMeta 不符合 "<namespace>/<name>" 格式的 SBC 时
- **THEN** Webhook 拒绝请求；格式在获取后端信息期间间接验证，返回错误 "unexpected key format: <key>"

#### Scenario:验证 SBC 更新 short-circuit 优化
- **WHEN** 用户更新 SBC 且 newClaim.Spec 与 oldClaim.Spec 相同且 newClaim.Annotations 与 oldClaim.Annotations 相同时
- **THEN** Webhook 跳过全部后续验证（包括 ConfigMap/Secret/后端登录检查），直接返回 nil 允许请求；注意：仅比较 Spec 和 Annotations，Labels 或 Finalizers 的单独变更也会触发 short-circuit 跳过验证

#### Scenario:验证 SBC 更新不更改不可变字段
- **WHEN** 用户更新 SBC 的 Provider 字段时
- **THEN** Webhook 拒绝请求，错误为 "[provider] is forbidden changed with StorageBackendClaim %s"；Provider 是唯一显式不可变的字段，ConfigMapMeta 和 SecretMeta 可变更但会触发完整重新验证（包括登录测试）

#### Scenario:验证配置有效的 SBC
- **WHEN** 用户创建带有所有必需字段和有效格式的 SBC 时
- **THEN** Webhook 执行完整的后端验证：检索 ConfigMap 和 Secret，构建后端对象，验证存储类型和插件存在性，验证参数，并执行到存储阵列的登录测试；如果全部通过，则允许请求

#### Scenario:验证 ConfigMap 存在性及 csi.json 键
- **WHEN** Webhook 在验证阶段检索 SBC 对应的 ConfigMap 时
- **THEN** 若 ConfigMap 不存在则拒绝请求；若 ConfigMap 的 Data 字段为 nil 则拒绝请求；注意：当前实现不显式检查 "csi.json" 键是否存在，缺失时 csi.json 返回 nil bytes，反序列化产生空结构体，导致下游错误 "parameters must be configured for backend" 而非明确的 "csi.json 键缺失" 错误

#### Scenario:验证 Secret 存在性及 user 字段
- **WHEN** Webhook 在验证阶段检索 SBC 对应的 Secret 时
- **THEN** 若 Secret 不存在则拒绝请求；若 Secret 的 Data 字段为 nil 则拒绝请求；注意：当前实现不显式检查 "user" 和 "password" 键是否存在，而是在后续登录验证阶段验证，错误为 "the \"user\" field in the secret does not exist or is empty" 或 "the \"password\" field in the secret does not exist or is empty"

#### Scenario:验证后端配置字段
- **WHEN** Webhook 构建 Backend 对象时
- **THEN** 若 ConfigMap 的 "csi.json" 中缺少 "storage" 字段或该字段未对应已注册的存储插件则拒绝请求；若缺少 "parameters" 字段则拒绝请求；若 "supportedTopologies" 格式无效（非列表、元素非字典、值非字符串）则拒绝请求；若 hyperMetro 配置不一致（配置了 metroDomain 或 metrovStorePairID 但未配置 metroBackend，或反之）则拒绝请求

#### Scenario:验证插件级协议和 Portal
- **WHEN** Webhook 执行登录验证时
- **THEN** 插件验证 protocol 字段是否为支持的协议类型（如 iSCSI、FC、NVMe 等），对需要 Portal 的协议验证 portals 字段存在且格式正确；authenticationMode（若存在）MUST 为 "local" 或 "ldap"

#### Scenario:活连接登录验证
- **WHEN** Webhook 执行存储阵列登录验证时
- **THEN** 插件建立与存储阵列的 TCP 连接，使用提供的凭据进行登录，登录成功后执行登出；若后端配置了多个 URL，连接失败时依次尝试所有 URL，非连接错误（如认证失败）立即返回；若所有 URL 均不可达或认证失败则拒绝请求

#### Scenario:验证 SBC 删除时 oldObject 解码失败
- **WHEN** 用户删除 SBC 时（Kubernetes DELETE 操作中 request.object 包含被删除对象、request.oldObject 为空）
- **THEN** Webhook 尝试解码空的 oldObject 失败，返回解码错误，导致所有 DELETE 请求在验证删除之前被拒绝；这是代码 bug，验证删除逻辑实际上不可达

#### Scenario:验证不带 finalizer 的 SBC 删除
- **WHEN** 用户删除不带任何 finalizer 的 SBC 时
- **THEN** Webhook 直接返回 nil（允许请求）

#### Scenario:验证带有 finalizer 的 SBC 删除
- **WHEN** 用户删除带有 ClaimBoundFinalizer 之外的 finalizer 的 SBC 时
- **THEN** Webhook 拒绝请求，错误为 "forbid delete StorageBackendClaim %s, there are some finalizers [%v]"；仅允许 ClaimBoundFinalizer（"storagebackend.xuanwu.huawei.io/storagebackendclaim-bound-protection"）；注意：由于 Delete 解码 bug，此场景在当前代码中不可达

#### Scenario:Webhook FailurePolicy 为 Fail
- **WHEN** Webhook 不可达时
- **THEN** 由于 FailurePolicy 设置为 Fail，所有 SBC 的 create/update/delete 操作 SHALL 被拒绝

#### Scenario:Webhook 适用于所有命名空间
- **WHEN** Webhook 创建 ValidatingWebhookConfiguration 时
- **THEN** 未设置 NamespaceSelector，Webhook SHALL 验证所有命名空间中的 StorageBackendClaim 资源

### Requirement: webhook-configuration SHALL manage webhook server and TLS setup
Webhook 服务器 SHALL 使用 TLS 加密通信，最低 TLS 版本为 1.2。当 TLS Secret 不存在时 SHALL 自动生成自签名证书。Webhook 配置 SHALL 使用 MatchPolicy=Exact、SideEffects=NoneOnDryRun，并支持 v1 和 v1beta1 两个 AdmissionReview 版本。

#### Scenario:自动生成 TLS 证书
- **WHEN** Webhook 启动时 TLS Secret 不存在时
- **THEN** Webhook 自动生成 X.509 证书（ECDSA P-521，SAN 仅包含服务 DNS 名称如 huawei-csi-controller.<namespace>.svc，无 IP SAN），创建 Secret 并使用该证书启动 HTTPS 服务器；注意：通过 IP 地址访问 webhook 时证书验证会失败

#### Scenario:使用最低 TLS 1.2 版本
- **WHEN** Webhook 启动 HTTPS 服务器时
- **THEN** TLS 配置 MUST 设置 MinVersion 为 TLS 1.2

#### Scenario:Webhook 配置协调
- **WHEN** Webhook 启动时创建或更新 ValidatingWebhookConfiguration 时
- **THEN** 若配置不存在则创建；若已存在且 webhook 定义不同则更新；若证书重新生成则用新 CA bundle 更新配置；名称为 storage-backend-controller.xuanwu.huawei.io

#### Scenario:Webhook 使用 Exact MatchPolicy
- **WHEN** Webhook 创建 ValidatingWebhookConfiguration 时
- **THEN** matchPolicy MUST 设置为 Exact

#### Scenario:Webhook 使用 NoneOnDryRun SideEffects
- **WHEN** Webhook 创建 ValidatingWebhookConfiguration 时
- **THEN** sideEffects MUST 设置为 NoneOnDryRun

#### Scenario:Webhook 支持双 AdmissionReview 版本
- **WHEN** Webhook 创建 ValidatingWebhookConfiguration 时
- **THEN** admissionReviewVersions MUST 包含 "v1" 和 "v1beta1"；注意：ValidatingWebhookConfiguration 配置中声明了 v1beta1 支持，但 scheme.go 仅注册了 admissionV1（未注册 admissionV1beta1），因此若 API Server 实际发送 v1beta1 AdmissionReview，UniversalDeserializer 无法解码，请求会在 Decode 阶段失败
