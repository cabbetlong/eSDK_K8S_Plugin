## Purpose

定义 CSI GetPluginInfo RPC 的接口规范，用于向容器编排器返回 CSI 驱动名称和供应商版本信息。

## Requirements

### Requirement: GetPluginInfo RPC 必须返回驱动标识
GetPluginInfo RPC SHALL 向 CO（容器编排器）返回 CSI 驱动名称和供应商版本。驱动名称在启动时配置，版本来自构建常量。

#### Scenario:CO 请求插件信息
- **WHEN** CO 发送 GetPluginInfoRequest 时
- **THEN** 驱动返回 GetPluginInfoResponse，包含驱动名称（来自启动配置）和供应商版本（来自构建常量）
