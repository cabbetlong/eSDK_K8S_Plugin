## Purpose

定义 CSI Probe RPC 的接口规范，用于作为 CSI 驱动的健康检查端点，验证驱动插件正在运行且状态健康。

## Requirements

### Requirement: Probe RPC 必须执行健康检查
Probe RPC SHALL 作为 CSI 驱动的健康检查端点。CO 使用它来验证驱动插件正在运行且健康。

#### Scenario:CO 探测驱动健康状态
- **WHEN** CO 发送 ProbeRequest 时
- **THEN** 驱动返回空的 ProbeResponse，指示插件健康且可运行
