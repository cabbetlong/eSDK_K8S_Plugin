## Purpose

定义 CSI NodeUnpublishVolume RPC 的接口规范，用于从节点目标路径移除已发布的卷，支持卸载和重试清理逻辑。

## Requirements

### Requirement: NodeUnpublishVolume RPC 必须从节点目标路径取消发布卷
NodeUnpublishVolume RPC SHALL 从节点的发布路径移除已发布的卷。如果目标路径已挂载，驱动必须卸载该路径，然后使用重试逻辑（最多 3 次尝试，间隔 1 秒）移除目标路径目录/文件。

#### Scenario:从节点取消发布已挂载的卷
- **WHEN** CO 发送带有 VolumeId 和 TargetPath 的 NodeUnpublishVolumeRequest，且 TargetPath 当前已挂载时
- **THEN** 驱动检查挂载路径是否存在，在 TargetPath 上执行 "umount"，以 1 秒间隔重试移除目标路径最多 3 次（忽略 "not exist" 错误），成功后返回空的 NodeUnpublishVolumeResponse

#### Scenario:取消发布未挂载的卷
- **WHEN** CO 发送带有 VolumeId 和 TargetPath 的 NodeUnpublishVolumeRequest，且 TargetPath 未挂载时
- **THEN** 驱动跳过卸载步骤，使用重试逻辑尝试移除目标路径，成功后返回空的 NodeUnpublishVolumeResponse

#### Scenario:目标路径不存在时取消发布卷
- **WHEN** CO 发送 NodeUnpublishVolumeRequest 且 TargetPath 已被移除时
- **THEN** 驱动将其视为成功（移除期间忽略 os.IsNotExist 错误），并返回空的 NodeUnpublishVolumeResponse

#### Scenario:拒绝检查挂载路径失败的 NodeUnpublishVolume
- **WHEN** CO 发送 NodeUnpublishVolumeRequest 且挂载状态检查失败时
- **THEN** 驱动返回 codes.Internal 错误，包含检查挂载路径失败原因

#### Scenario:拒绝卸载失败的 NodeUnpublishVolume
- **WHEN** CO 发送 NodeUnpublishVolumeRequest，TargetPath 已挂载，且 umount 命令执行失败（返回结果不包含 "not mount"）时
- **THEN** 驱动返回 codes.Internal 错误，包含卸载失败原因

#### Scenario:拒绝移除目标路径失败的 NodeUnpublishVolume
- **WHEN** CO 发送 NodeUnpublishVolumeRequest 且重试 3 次（间隔 1 秒）后仍无法移除目标路径时
- **THEN** 驱动返回 codes.Internal 错误，包含删除目标路径失败原因

#### Scenario:umount 返回 "not mounted" 时容忍
- **WHEN** CO 发送 NodeUnpublishVolumeRequest 且 umount 命令失败但输出包含 "not mounted" 时
- **THEN** 驱动将 "not mounted" 错误视为成功，继续执行目标路径移除（幂等行为）

#### Scenario:umount 命令通过 nsenter 在宿主命名空间执行
- **WHEN** CO 发送 NodeUnpublishVolumeRequest 且目标路径已挂载时
- **THEN** 驱动通过 nsenter 进入宿主的 IPC/MNT/NET/UTS 命名空间执行 umount 命令，确保在正确的挂载命名空间中操作

#### Scenario:umount 命令超时杀进程组
- **WHEN** CO 发送 NodeUnpublishVolumeRequest 且 umount 命令超过执行超时时间（默认 30 秒）时
- **THEN** 驱动向 umount 进程组发送 SIGKILL 终止整个进程组，并返回超时错误

#### Scenario:读取 /proc/mounts 一致性重试
- **WHEN** CO 发送 NodeUnpublishVolumeRequest 且检查挂载状态时
- **THEN** 驱动读取 /proc/mounts 时使用一致性读取，最多重试 10 次以获取稳定快照，防止内核修改文件导致读取不一致

#### Scenario:非空目录无法移除
- **WHEN** CO 发送 NodeUnpublishVolumeRequest 且目标路径为非空目录时
- **THEN** 驱动重试 os.Remove 3 次后仍收到 "directory not empty" 错误，返回 codes.Internal 错误

#### Scenario:设备忙无法移除
- **WHEN** CO 发送 NodeUnpublishVolumeRequest 且目标路径仍被挂载（设备忙）时
- **THEN** 驱动重试 os.Remove 3 次后仍收到 "device or resource busy" 错误，返回 codes.Internal 错误

#### Scenario:符号链接目标路径可被移除
- **WHEN** CO 发送 NodeUnpublishVolumeRequest 且目标路径是符号链接时
- **THEN** 驱动通过 os.Remove 成功移除符号链接

#### Scenario:umount 超时返回错误
- **WHEN** CO 发送 NodeUnpublishVolumeRequest 且 umount 命令在超时时间内未完成时
- **THEN** 驱动返回 codes.Internal 错误，错误消息为 "timeout"
