## Purpose

定义 CSI NodeGetInfo RPC 的接口规范，用于向容器编排器返回节点标识信息、主机名和拓扑约束数据。

## Requirements

### Requirement: NodeGetInfo RPC 必须返回节点标识和拓扑
NodeGetInfo RPC SHALL 向 CO 返回节点的标识信息。NodeId 是一个包含 HostName 的 JSON 编码字符串。如果节点的标签中有拓扑信息，则将其作为 AccessibleTopology 包含在响应中。

#### Scenario:获取不带拓扑的节点信息
- **WHEN** CO 发送 NodeGetInfoRequest 且驱动的 nodeName 为空（未通过 CSI_NODENAME 配置）时
- **THEN** 驱动获取主机名，将其编组为 JSON {"HostName": "<hostname>"}，并返回 NodeGetInfoResponse，包含 NodeId（JSON 字符串）、MaxVolumesPerNode（来自启动配置的 MaxVolumesPerNode），不包含 AccessibleTopology

#### Scenario:获取带拓扑的节点信息
- **WHEN** CO 发送 NodeGetInfoRequest 且驱动的 nodeName 已配置（通过 CSI_NODENAME 环境变量）时
- **THEN** 驱动获取主机名，从节点标签中获取拓扑段（这些是带有拓扑前缀的标签），并返回 NodeGetInfoResponse，包含 NodeId、MaxVolumesPerNode 和包含拓扑段的 AccessibleTopology

#### Scenario:拒绝获取主机名失败的 NodeGetInfo
- **WHEN** CO 发送 NodeGetInfoRequest 且无法获取主机名时
- **THEN** 驱动返回 codes.Internal 错误

#### Scenario:拒绝获取拓扑失败的 NodeGetInfo
- **WHEN** CO 发送 NodeGetInfoRequest（nodeName 已配置）且无法获取拓扑信息时
- **THEN** 驱动返回 codes.Internal 错误

#### Scenario:拒绝节点信息编组失败的 NodeGetInfo
- **WHEN** CO 发送 NodeGetInfoRequest 且节点信息序列化失败时
- **THEN** 驱动返回 codes.Internal 错误
