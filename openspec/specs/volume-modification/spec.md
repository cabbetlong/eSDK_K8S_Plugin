## Purpose

定义通过 VolumeModifyClaim 和 VolumeModifyContent CRD 批量修改卷属性的规范，当前实现双活转换功能，包括 VMC 生命周期、VContent 处理、DR-CSI ModifyVolume 服务调用和进度跟踪。

## Requirements

### Requirement: volume-modification SHALL batch modify volume attributes
卷修改系统 SHALL 允许通过 VolumeModifyClaim（VMC）和 VolumeModifyContent CRD 批量修改卷属性（目前仅实现了双活转换）。volume-modify-controller 处理 VMC，为匹配的 PV 创建 VContent，调用 DR-CSI ModifyVolume gRPC 应用更改，跟踪进度，并在所有修改完成后替换 StorageClass。

#### Scenario: 卷修改系统初始化
- **WHEN** CSI 驱动启动时
- **THEN** 系统 SHALL 注册 VolumeModifyClaim 和 VolumeModifyContent CRD，并启动 volume-modify-controller 监听卷修改请求

---

### Requirement: claim-processing SHALL process VolumeModifyClaim lifecycle
volume-modify-controller SHALL 监听 VolumeModifyClaim 资源，基于 Source（StorageClass）查找匹配的 PersistentVolume，为每个匹配的 PV 创建 VolumeModifyContent 资源，跟踪总体进度，并在所有修改完成后替换 StorageClass。目前仅实现了双活修改。

#### Scenario:处理新的 VolumeModifyClaim
- **WHEN** 创建新的 VMC，Source.Kind="StorageClass" 且 Source.Name=<sc-name> 时
- **THEN** 控制器验证 Source.Kind 等于 "StorageClass"，验证 StorageClass 存在且其 Provisioner 匹配控制器的 provisioner（默认 "csi.huawei.com"），验证 Parameters 不为空且仅包含支持的键（hyperMetro、metroPairSyncSpeed），查找使用指定 StorageClass 的所有 PV（无命名空间过滤），为每个 PV 使用 VMC 的 Parameters 创建 VolumeModifyContent，将 VMC Phase 设置为 "Creating"，并记录 StartedAt 时间戳

#### Scenario:处理带源命名空间的 VMC（未实现）
- **WHEN** 创建指定了 Source.Namespace 的 VMC 时
- **THEN** 控制器不按命名空间过滤 PV；它迭代集群中的所有 PV，不考虑 Source.Namespace 字段

#### Scenario:更新 VMC 进度
- **WHEN** 正在处理 VContent 时
- **THEN** 控制器更新 VMC 的 Ready 字段（例如 "2/5"）和 Contents 数组，包含每个 Content 的名称、源卷和状态；Ready 格式为 "completed/total"，其中 completed 统计处于 "Completed" 阶段的 Content 数量

#### Scenario:所有 Content 完成时完成 VMC
- **WHEN** 所有关联的 VContent 达到 "Completed" 状态时
- **THEN** 控制器替换 StorageClass（删除并使用更新后的参数重新创建），创建备份 StorageClass（<scName>-<claimName>）以保证安全，将 VMC Phase 设置为 "Completed"，并记录 CompletedAt 时间戳

#### Scenario:删除时回滚 VMC
- **WHEN** 处于 "Creating" 阶段的 VMC 被删除时
- **THEN** 控制器将 VMC Phase 设置为 "Rollback"，为所有关联的 VContent 添加注释 modify.xuanwu.io/reclaimPolicy=rollback，删除 VContent，等待所有 VContent 完成回滚，然后将 Phase 设置为 "Deleting" 并移除 finalizer

#### Scenario:处理没有匹配 PV 的 VMC
- **WHEN** 创建 VMC 但没有 PV 匹配源 StorageClass 时
- **THEN** 控制器立即将 VMC Phase 设置为 "Completed"，Ready="0/0"

#### Scenario:处理带 finalizer 的 VMC 删除
- **WHEN** 处于 "Creating" 或 "Completed" 阶段的 VMC 被删除时
- **THEN** 控制器通过 finalizer 处理删除，如果处于 "Creating" 则将 Phase 设置为 "Rollback"（触发所有 Content 的回滚），等待 VContent 完成回滚，然后移除 finalizer 并删除 VMC

#### Scenario:处理 VMC 时对失败的 Content 使用指数退避
- **WHEN** VolumeModifyContent 在修改期间失败时
- **THEN** 控制器使用指数退避重试（baseDelay=5s，maxDelay=5min，延迟序列：5s、10s、20s、40s、80s、160s、300s 封顶），直到 Content 成功或 VMC 被删除

#### Scenario:没有 Content 需要处理时跳过 VMC 处理
- **WHEN** VMC 调和循环运行且所有关联的 VContent 处于 "Completed" 状态时
- **THEN** 控制器将 VMC Phase 更新为 "Completed"，并记录 CompletedAt 时间戳

#### Scenario:验证 VMC 的双活参数
- **WHEN** 创建包含 hyperMetro 的 Parameters 的 VMC 时
- **THEN** 控制器验证 hyperMetro 值恰好为 "true"；如果还指定了 metroPairSyncSpeed，则验证其可解析为 [1, 4] 范围内的整数；如果验证失败则拒绝并返回错误

#### Scenario:拒绝带不支持的修改类型的 VMC
- **WHEN** 创建包含 hyperMetro 或 metroPairSyncSpeed 之外的键的 Parameters 的 VMC 时
- **THEN** 控制器拒绝请求，错误指示不支持的参数；目前仅实现了双活修改

#### Scenario:所有修改完成后替换 StorageClass
- **WHEN** 所有 VContent 达到 "Completed" 状态时
- **THEN** 控制器检查 StorageClass 参数是否已包含 VMC 参数；如果不包含，则创建备份 StorageClass（<scName>-<claimName>），删除原始 StorageClass，并使用合并的参数重新创建它；容忍备份 SC 创建期间的 AlreadyExists 错误和原始 SC 删除期间的 NotFound 错误

#### Scenario:父 claim 删除时取消 Content 处理
- **WHEN** Content worker 正在处理 VolumeModifyContent 且父 VMC 被删除时
- **THEN** 驱动检测到父 claim 删除并取消 Content 处理

---

### Requirement: content-processing SHALL execute volume modifications via DR-CSI
volume-modify-controller SHALL 通过调用 DR-CSI ModifyVolume gRPC 服务处理每个 VolumeModifyContent，将修改应用到存储阵列。目前仅实现了双活修改（Local2HyperMetro 和 HyperMetro2Local）。

#### Scenario:处理 VolumeModifyContent
- **WHEN** VolumeModifyContent 处于 "Pending" 阶段时
- **THEN** 控制器验证 Parameters 不为空，验证 hyperMetro 参数（必须为 "true" 或 "false"），如果存在则验证 metroPairSyncSpeed（[1, 4] 范围内的整数），连接到 DR-CSI 提供方，使用卷 ID、StorageClassParameters 和 MutableParameters 调用 ModifyVolume，并将 Content 状态更新为 "Creating"（不是 "InProgress"）

#### Scenario:修改成功时完成 Content
- **WHEN** DR-CSI ModifyVolume 调用成功时
- **THEN** 控制器将 Content Phase 设置为 "Completed"，状态更新最多重试 10 次，间隔 100ms

#### Scenario:重试失败的 Content 修改
- **WHEN** DR-CSI ModifyVolume 调用失败时
- **THEN** 控制器记录 Warning 事件，返回错误用于限速重新入队（指数退避 5s-5min）；下次重试时，在更新状态之前，驱动验证（VolumeHandle、VolumeModifyClaimName、SourceVolume、Parameters、StorageClassParameters）均未更改；如果任何一项已更改，则中止重试

#### Scenario:状态更新失败时回滚 Content
- **WHEN** 驱动在 10 次重试后无法将 Content 状态更新为 "Completed" 时
- **THEN** 控制器执行回滚，发送带有 hyperMetro="false" 的 ModifyVolume gRPC 调用以还原修改

#### Scenario:VMC 删除时回滚 Content
- **WHEN** VMC 正在删除且其 Content 处于 "Creating" 阶段时
- **THEN** 控制器为 Content 添加注释 modify.xuanwu.io/reclaimPolicy=rollback，控制器检测到注释，将 Content Phase 设置为 "Rollback"，执行回滚（设置 hyperMetro="false"），并在回滚完成后移除 finalizer

#### Scenario:回滚 Content 时仅反转双活参数
- **WHEN** 为 Content 执行回滚时
- **THEN** 回滚参数仅包含 hyperMetro="false"（其他修改类型如 QoS、SmartTier、SmartMigration 不支持回滚）

#### Scenario:VMC 处于 Rollback 阶段时跳过 Content 处理
- **WHEN** 控制器调和处于 "Rollback" 阶段的 VMC 时
- **THEN** 它取消进行中的修改，并等待所有 Content 完成回滚，然后再转换到 "Deleting"

#### Scenario:处理修改期间的 DR-CSI 连接失败
- **WHEN** 控制器在 ModifyVolume 调用期间无法连接到 DR-CSI gRPC 服务器时
- **THEN** 错误返回用于限速重新入队；Content 保持 "Creating" 阶段，并将在下一个调和周期重试

#### Scenario:在提供方级别验证 ModifyVolume 参数
- **WHEN** DR-CSI 提供方收到 ModifyVolume 请求时
- **THEN** 驱动验证 hyperMetro 值为 "true" 或 "false"，选择后端，验证已配置 MetroBackend，选择远程池，并在后端上执行修改；对于 OceanStor NAS 类型验证本地和远程 WWN 均非空，设备不同，且逻辑端口在本地站点运行

#### Scenario:父 claim 删除时 Content 同步取消
- **WHEN** Content worker 正在处理 VolumeModifyContent 且父 VMC 被删除时
- **THEN** 驱动检查父 claim 是否存在；如果未找到，则提前返回而不处理修改

---

### Requirement: DR-CSI ModifyVolume service SHALL modify volume attributes
DR-CSI ModifyVolume gRPC 服务 SHALL 提供卷修改操作。目前仅实现了双活转换（Local2HyperMetro 和 HyperMetro2Local）。

#### Scenario:修改卷双活（Local 到 HyperMetro）
- **WHEN** volume-modify-controller 通过 DR-CSI 调用 ModifyVolume，MutableParameters 包含 hyperMetro="true" 时
- **THEN** 驱动验证 hyperMetro 值，选择后端，验证已配置 MetroBackend，选择远程池，并在后端上执行 Local2HyperMetro 类型修改；对于 OceanStor NAS 类型验证本地和远程 WWN 均非空，设备不同，且逻辑端口在本地站点运行

#### Scenario:修改卷双活（HyperMetro 到 Local）
- **WHEN** volume-modify-controller 通过 DR-CSI 调用 ModifyVolume，MutableParameters 包含 hyperMetro="false" 时
- **THEN** 驱动使用 HyperMetro2Local 类型处理请求，将卷从双活 active-active 转换为仅本地模式

#### Scenario:拒绝 hyperMetro 值无效的 ModifyVolume
- **WHEN** volume-modify-controller 调用 ModifyVolume，MutableParameters 包含的 hyperMetro 设置为 "true" 或 "false" 之外的值时
- **THEN** 驱动返回错误 "hyperMetro value must be \"true\" or \"false\", \"X\" is invalid."

#### Scenario:拒绝未配置双活后端的 ModifyVolume
- **WHEN** 使用 hyperMetro="true" 调用 ModifyVolume 但后端的 MetroBackend 为 nil 时
- **THEN** 服务返回错误 "have not configured hyper metro backend"

#### Scenario:拒绝远程池选择失败的 ModifyVolume
- **WHEN** 调用 ModifyVolume 且 SelectRemotePool 失败（无双活后端、无兼容池或双活+远程复制冲突）时
- **THEN** 服务返回错误 "select remote pool failed, backend name: X, error: Y"

#### Scenario:修改卷时对无法识别的参数执行空操作
- **WHEN** 调用 ModifyVolume，MutableParameters 不包含 "hyperMetro" 键时
- **THEN** 驱动立即返回空响应而不执行任何修改

#### Scenario:拒绝不支持的存储类型上的 ModifyVolume
- **WHEN** 在 oceandisk-san、oceanstor-a-series-nas、oceanstor-a-series-nas-dme 或 oceanstor-dtree 后端上调用 ModifyVolume 时
- **THEN** 插件返回错误，指示存储类型不支持卷修改功能
