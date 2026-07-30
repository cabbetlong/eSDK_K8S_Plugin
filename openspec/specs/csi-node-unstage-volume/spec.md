## Purpose

定义 CSI NodeUnstageVolume RPC 的接口规范，用于从节点暂存目标路径移除已暂存的卷，支持 SAN 断开连接和 NAS 卸载操作。

## Requirements

### Requirement: NodeUnstageVolume RPC 必须从节点取消暂存卷
NodeUnstageVolume RPC SHALL 从节点的暂存目标路径移除已暂存的卷。驱动为后端创建 manage.Manager 并委托取消暂存操作。对于 SAN 卷，驱动必须检索设备 WWN（从磁盘文件或目标路径），卸载暂存路径，断开卷连接，并清理 WWN 文件。对于 NAS 卷，仅需卸载。DTree 卷不需要暂存/取消暂存。

#### Scenario:取消暂存 SAN 卷
- **WHEN** CO 发送带有 VolumeId 和 StagingTargetPath 的 NodeUnstageVolumeRequest 时
- **THEN** 驱动分割 VolumeId 获取 backendName，为 SAN 后端创建管理器，检索设备 WWN（首先从 WWN 文件获取，如果文件不存在则通过 /proc/mounts 从目标路径获取），如果从目标路径获取则将 WWN 写入磁盘（用于幂等重试），检查裸块暂存路径是否存在并相应卸载，按 WWN 断开卷连接，移除 WWN 文件，成功后返回空的 NodeUnstageVolumeResponse

#### Scenario:WWN 检索失败时取消暂存 SAN 卷
- **WHEN** CO 发送 NodeUnstageVolumeRequest 且无法检索设备 WWN（文件不存在且目标路径不包含 WWN 信息）时
- **THEN** 驱动记录警告并返回成功（幂等——没有 WWN 重试不太可能有帮助）

#### Scenario:取消暂存 NAS 卷
- **WHEN** CO 发送 NAS 后端上卷的 NodeUnstageVolumeRequest 时
- **THEN** 驱动创建 NasManager；对于 DTree 存储，立即返回（不需要取消暂存）；对于其他 NAS 类型，卸载暂存目标路径并返回空的 NodeUnstageVolumeResponse

#### Scenario:已卸载时取消暂存 NAS 卷
- **WHEN** CO 发送 NodeUnstageVolumeRequest 且暂存目标路径已卸载时
- **THEN** 驱动返回成功（幂等行为）

#### Scenario:拒绝卸载失败的 NodeUnstageVolume
- **WHEN** CO 发送 NodeUnstageVolumeRequest 且卸载操作失败时
- **THEN** 驱动返回 codes.Internal 错误，包含卸载失败原因

#### Scenario:从目标路径回退检索 WWN 取消暂存 SAN 卷
- **WHEN** CO 发送 NodeUnstageVolumeRequest 且 WWN 文件在预期路径不存在时
- **THEN** 驱动读取 /proc/mounts 以查找与暂存目标路径关联的设备，从设备路径提取 WWN，将 WWN 写入磁盘用于幂等重试，然后继续取消暂存

#### Scenario:检测裸块暂存路径取消暂存 SAN 卷
- **WHEN** CO 发送块卷的 NodeUnstageVolumeRequest 时
- **THEN** 驱动检查暂存目标路径是否作为文件存在（裸块绑定挂载），并在断开卷连接之前相应地卸载它

#### Scenario:并发取消暂存同一卷（WWN 同步锁）
- **WHEN** 同一节点上多个请求同时取消暂存同一卷（相同 WWN）时
- **THEN** 驱动通过 WWN 级文件锁和信号量串行化断开连接操作，确保同一时间只有一个协程操作同一卷

#### Scenario:同步锁获取超时
- **WHEN** CO 发送 NodeUnstageVolumeRequest 且 WWN 同步锁在 30 秒内无法获取时
- **THEN** 驱动返回错误，指示获取断开连接同步锁超时

#### Scenario:iSCSI 断开连接重试
- **WHEN** CO 发送 iSCSI 后端上卷的 NodeUnstageVolumeRequest 且断开连接后仍有残留设备时
- **THEN** 驱动以 1 秒间隔重试断开操作最多 1 分钟，直到所有设备已移除或返回真实错误

#### Scenario:iSCSI Session 登出
- **WHEN** CO 发送 iSCSI 后端上卷的 NodeUnstageVolumeRequest 且设备已移除时
- **THEN** 驱动将 iSCSI 节点启动模式设为 manual，执行 `iscsiadm --logout` 登出会话，然后执行 `iscsiadm --op delete` 删除节点记录

#### Scenario:iSCSI Session 已断开容忍
- **WHEN** CO 发送 iSCSI 后端上卷的 NodeUnstageVolumeRequest 且 iSCSI Session 已处于断开状态时
- **THEN** 驱动容忍退出码 0、15、255，将其视为成功登出，不返回错误

#### Scenario:FC 断开连接（无 Session 登出）
- **WHEN** CO 发送 FC 后端上卷的 NodeUnstageVolumeRequest 时
- **THEN** 驱动移除设备路径但不执行 Session 登出操作（FC 协议无 iSCSI 式会话管理）

#### Scenario:NVMe/FC-NVMe 断开连接
- **WHEN** CO 发送 NVMe 或 FC-NVMe 后端上卷的 NodeUnstageVolumeRequest 时
- **THEN** 驱动查找 NVMe 物理设备和会话端口，移除设备，执行 `nvme disconnect` 断开会话；若使用多路径则等待 3 秒后 Flush DM 设备

#### Scenario:NVMe 设备已断开直接成功
- **WHEN** CO 发送 NVMe/FC-NVMe 后端上卷的 NodeUnstageVolumeRequest 且设备已不存在时
- **THEN** 驱动直接返回成功，不进入重试循环（与 iSCSI/FC 的 "FindNoDevice" 重试机制不同）

#### Scenario:Local SCSI 断开连接为空操作
- **WHEN** CO 发送本地 SCSI 后端上卷的 NodeUnstageVolumeRequest 时
- **THEN** 驱动不执行任何断开连接操作，直接返回成功

#### Scenario:DM 设备 Flush 重试
- **WHEN** CO 发送使用 DM-Multipath 的 SAN 卷的 NodeUnstageVolumeRequest 且 DM 设备 Flush 失败时
- **THEN** 驱动重试 `multipath -f` 最多 3 次，每次间隔 20 秒；全部失败则返回错误

#### Scenario:多路径设备移除策略
- **WHEN** CO 发送 SAN 卷的 NodeUnstageVolumeRequest 且设备使用多路径时
- **THEN** 驱动根据多路径类型执行不同的移除策略：DM-Multipath 先 Flush DM 设备再移除子设备；UltraPath 先刷 IO 再删除物理和虚拟设备；UltraPathNVMe 先设置 IO 挂起时间为 0 再清理

#### Scenario:设备已删除视为成功
- **WHEN** CO 发送 SAN 卷的 NodeUnstageVolumeRequest 且物理设备已从系统中删除时
- **THEN** 驱动检测到 "No such file or directory" 错误后视为成功，不返回错误

#### Scenario:WWN 文件删除失败不阻断
- **WHEN** CO 发送 SAN 卷的 NodeUnstageVolumeRequest 且卷已成功断开连接但 WWN 文件删除失败时
- **THEN** 驱动仅记录错误日志，仍返回成功（断开连接为权威操作）

#### Scenario:WWN 文件不存在视为成功
- **WHEN** CO 发送 SAN 卷的 NodeUnstageVolumeRequest 且 WWN 文件在预期路径不存在时
- **THEN** 驱动将 WWN 文件不存在视为已清理，不返回错误

#### Scenario:卸载时 "not mounted" 容忍
- **WHEN** CO 发送 NodeUnstageVolumeRequest 且 umount 命令返回 "not mounted" 或 "not found" 错误时
- **THEN** 驱动将其视为成功，不返回错误（幂等行为）

#### Scenario:NAS 卸载后清理目标路径
- **WHEN** CO 发送 NAS 卷的 NodeUnstageVolumeRequest 且卸载成功时
- **THEN** 驱动在卸载后删除暂存目标路径目录

#### Scenario:同一 WWN 存在多个虚拟设备报错
- **WHEN** CO 发送 SAN 卷的 NodeUnstageVolumeRequest 且同一 WWN 在 /dev/disk/by-id/ 下存在多个虚拟设备时
- **THEN** 驱动返回错误 "virtual device not unique"

#### Scenario:从目标路径获取 WWN 时多挂载引用报错
- **WHEN** CO 发送 NodeUnstageVolumeRequest 且 WWN 文件不存在，需从目标路径获取 WWN，但设备被多个挂载路径引用时
- **THEN** 驱动返回错误，指示设备被多个挂载路径引用

#### Scenario:断开连接时跳过分区设备
- **WHEN** CO 发送 SAN 卷的 NodeUnstageVolumeRequest 且 /dev/disk/by-id/ 下存在分区设备时
- **THEN** 驱动在查找虚拟设备时跳过分区设备（尾随数字的设备名如 sdc1、nvme0n1p1）

#### Scenario:WWN 文件名截断
- **WHEN** CO 发送 SAN 卷的 NodeUnstageVolumeRequest 且 VolumeId 长度超过 64 字符时
- **THEN** 驱动截取 VolumeId 末尾 64 字符作为 WWN 文件名
