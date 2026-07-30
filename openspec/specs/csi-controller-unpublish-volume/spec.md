## Purpose

定义 CSI ControllerUnpublishVolume RPC 的接口规范，用于从华为存储阵列上将卷从节点分离，支持多种存储类型、双活配置和幂等操作。

## Requirements

### Requirement: ControllerUnpublishVolume RPC 必须从节点分离卷
ControllerUnpublishVolume RPC SHALL 在华为存储阵列上将卷从特定节点分离（取消发布）。如果后端不再存在，请求返回成功并带有警告（幂等行为）。对于 DTree 卷，驱动必须在分离参数中包含 parentName。对于双活卷，驱动必须从两个站点分别分离。

#### Scenario:从节点取消发布卷
- **WHEN** CO 发送带有 VolumeId 和 NodeId 的 ControllerUnpublishVolumeRequest 时
- **THEN** 驱动分割 VolumeId 获取 backendName 和 volName，解组 NodeId JSON 提取节点参数，在后端上分离卷，成功后返回空的 ControllerUnpublishVolumeResponse

#### Scenario:从节点取消发布 DTree 卷
- **WHEN** CO 发送 DTree 存储后端上卷的 ControllerUnpublishVolumeRequest 时
- **THEN** 驱动从卷 ID 映射中获取 DTree parentName，将其添加到分离参数中，并在后端上分离卷

#### Scenario:后端不存在时取消发布卷（幂等）
- **WHEN** CO 发送 VolumeId 引用不存在后端的 ControllerUnpublishVolumeRequest 时
- **THEN** 驱动记录警告，返回成功并带有空的 ControllerUnpublishVolumeResponse，并注明需要从存储阵列手动分离

#### Scenario:拒绝 nodeId JSON 无效的 ControllerUnpublishVolume
- **WHEN** CO 发送 NodeId 无法解组为 JSON 的 ControllerUnpublishVolumeRequest 时
- **THEN** 驱动返回 codes.Internal 错误，包含解组错误消息

#### Scenario:LUN 不存在时取消发布卷（幂等）
- **WHEN** CO 发送 OceanStor SAN 后端上卷的 ControllerUnpublishVolumeRequest，且存储阵列上该 LUN 已不存在时
- **THEN** 附加器记录信息日志指示 LUN 不存在，返回空 WWN 和 nil 错误，驱动返回空的 ControllerUnpublishVolumeResponse

#### Scenario:主机不存在时取消发布卷（幂等）
- **WHEN** CO 发送 OceanStor SAN 或 OceanDisk 后端上卷的 ControllerUnpublishVolumeRequest，且存储阵列上该主机已不存在时
- **THEN** 附加器记录信息日志指示主机不存在，返回空结果和 nil 错误，驱动返回空的 ControllerUnpublishVolumeResponse

#### Scenario:后端插件分离卷失败
- **WHEN** CO 发送 ControllerUnpublishVolumeRequest 且后端分离卷操作返回错误时
- **THEN** 驱动返回 codes.Internal 错误，包含插件返回的错误消息

#### Scenario:取消发布双活卷（双站点正常）
- **WHEN** CO 发送启用双活的 OceanStor SAN 后端上卷的 ControllerUnpublishVolumeRequest，且本地和远程存储均在线时
- **THEN** MetroAttacher 先调用远端 ControllerDetach，再调用本地 ControllerDetach，合并两个站点的 LUN WWN，成功后返回空的 ControllerUnpublishVolumeResponse

#### Scenario:取消发布双活卷时远端分离失败
- **WHEN** CO 发送启用双活的 OceanStor SAN 后端上卷的 ControllerUnpublishVolumeRequest，且 MetroAttacher 远端站点分离失败时
- **THEN** 驱动返回错误，且不尝试本地站点的分离操作

#### Scenario:取消发布双活卷时远端成功但本地失败
- **WHEN** CO 发送启用双活的 OceanStor SAN 后端上卷的 ControllerUnpublishVolumeRequest，且 MetroAttacher 远端站点分离成功但本地站点分离失败时
- **THEN** 驱动返回错误，远端站点的分离结果不回滚（远端 LUN 映射已被移除）

#### Scenario:双活 pair 处于暂停状态时允许取消发布
- **WHEN** CO 发送启用双活的 OceanStor SAN 后端上卷的 ControllerUnpublishVolumeRequest，且 HyperMetro pair 的 RUNNINGSTATUS 为暂停状态时
- **THEN** 驱动记录警告日志，继续执行分离操作（与 ControllerAttach 不同，ControllerDetach 允许暂停状态的 pair）

#### Scenario:取消发布双活卷时双站点均离线
- **WHEN** CO 发送启用双活的 OceanStor SAN 后端上卷的 ControllerUnpublishVolumeRequest，且本地和远程存储均离线时
- **THEN** 驱动返回错误，指示本地和远程存储均不在线

#### Scenario:取消发布双活卷时仅本地在线（回退）
- **WHEN** CO 发送启用双活的 OceanStor SAN 后端上卷的 ControllerUnpublishVolumeRequest，且仅本地存储在线时
- **THEN** 驱动记录警告，回退使用本地插件执行单站点分离操作

#### Scenario:取消发布双活卷时仅远程在线（回退）
- **WHEN** CO 发送启用双活的 OceanStor SAN 后端上卷的 ControllerUnpublishVolumeRequest，且仅远程存储在线时
- **THEN** 驱动记录警告，回退使用远程插件执行单站点分离操作

#### Scenario:DTree parentName 缺失时分离失败
- **WHEN** CO 发送 OceanStor DTree 存储后端上卷的 ControllerUnpublishVolumeRequest，且分离参数中缺少 DTree parentName 时
- **THEN** 插件返回错误，指示无法获取 DTree 的 parentName

#### Scenario:NFS 自动认证客户端移除访问权限
- **WHEN** CO 发送启用 nfsAutoAuthClient 的 NAS 卷的 ControllerUnpublishVolumeRequest 时
- **THEN** 驱动从 Secret 获取节点主机 IP，按 CIDR 过滤，调用 AutoManageAuthClient 将匹配的 NFS 客户端访问权限设置为 NoAccess

#### Scenario:NFS 共享不存在时设置无访问权限（幂等）
- **WHEN** CO 发送启用 nfsAutoAuthClient 的 NAS 卷的 ControllerUnpublishVolumeRequest，且存储阵列上该 NFS 共享已不存在时
- **THEN** AutoManageAuthClient 记录信息日志指示共享已无访问权限，返回 nil（幂等成功）

#### Scenario:DTree 卷 NFS 自动认证客户端移除访问权限
- **WHEN** CO 发送启用 nfsAutoAuthClient 的 DTree 卷的 ControllerUnpublishVolumeRequest 时
- **THEN** 驱动从 Secret 获取节点主机 IP，按 CIDR 过滤，使用 parentName 和 volName 构造共享路径，调用 AutoManageAuthClient 将匹配的 NFS 客户端访问权限设置为 NoAccess

#### Scenario:IO 隔离模式下验证客户端状态
- **WHEN** CO 发送启用 nfsAutoAuthClient 且 IOIsolation 参数为 true 的 NAS/DTree 卷的 ControllerUnpublishVolumeRequest 时
- **THEN** 驱动在设置 NoAccess 后调用 CheckAllClientsStatus 轮询验证所有客户端访问状态已变为预期值，直到超时或全部确认

#### Scenario:OceanDisk 命名空间不存在时取消发布卷（幂等）
- **WHEN** CO 发送 OceanDisk SAN 后端上卷的 ControllerUnpublishVolumeRequest，且存储阵列上该命名空间已不存在时
- **THEN** 插件记录警告日志指示命名空间不存在，返回 nil（幂等成功）

#### Scenario:FusionStorage SAN 取消发布卷
- **WHEN** CO 发送 FusionStorage SAN 后端上卷的 ControllerUnpublishVolumeRequest 时
- **THEN** 驱动使用 FusionStorage 专用附加器执行 ControllerDetach，从主机移除 LUN 映射并返回 LUN WWN

#### Scenario:FusionStorage NAS/DTree 取消发布卷为空操作
- **WHEN** CO 发送 FusionStorage NAS 或 FusionStorage DTree 后端上卷的 ControllerUnpublishVolumeRequest 时
- **THEN** 插件返回 nil（空操作，无需存储侧分离）

#### Scenario:A-Series/DME 取消发布卷为空操作
- **WHEN** CO 发送 OceanStor A-Series NAS、A-Series DTree 或 DME 后端上卷的 ControllerUnpublishVolumeRequest 时
- **THEN** 插件返回 nil（空操作，无需存储侧分离）

#### Scenario:HyperMetro 检测 JSON 解析失败时按非双活处理
- **WHEN** CO 发送 OceanStor SAN 后端上卷的 ControllerUnpublishVolumeRequest，且 LUN 的 HASRSSOBJECT 字段 JSON 格式无效时
- **THEN** 驱动将卷视为非双活卷，使用本地插件执行单站点分离操作
