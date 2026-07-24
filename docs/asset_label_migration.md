# Asset 指标去重变更映射

本文档记录 `siliconflow_asset_*` 系列指标中移除的与 node_exporter 默认采集器重复的部分，以及对应的替代指标。

---

## 1. siliconflow_asset_machine_info

**变更说明：** 移除了与 `dmi`、`uname`、`os` 采集器重复的标签，仅保留独有字段。

### 新指标格式

```
siliconflow_asset_machine_info{uuid="<uuid>", type="<physical|virtual>", k8s_node="<true|false>"} 1
```

### 旧标签 → 替代指标映射

| 旧标签 | 替代指标 | 替代标签 | 采集器 |
|---|---|---|---|
| `vendor` | `node_dmi_info` | `system_vendor` | dmi |
| `product` | `node_dmi_info` | `product_name` | dmi |
| `version` | `node_dmi_info` | `product_version` | dmi |
| `serial` | `node_dmi_info` | `product_serial` | dmi |
| `machine_uuid` | `node_dmi_info` | `product_uuid` | dmi |
| `hostname` | `node_uname_info` | `nodename` | uname |
| `kernel` | `node_uname_info` | `release` | uname |
| `kernel_arch` | `node_uname_info` | `machine` | uname |
| `os` | `node_os_info` | `name` | os |
| `os_version` | `node_os_info` | `version_id` | os |
| `board_vendor` | `node_dmi_info` | `board_vendor` | dmi |
| `board_name` | `node_dmi_info` | `board_name` | dmi |
| `board_version` | `node_dmi_info` | `board_version` | dmi |
| `board_serial` | `node_dmi_info` | `board_serial` | dmi |

### 保留的标签

| 标签 | 说明 | 为何保留 |
|---|---|---|
| `uuid` | 机器唯一标识 | asset 采集器的关联键 |
| `type` | 物理机/虚拟机 | 默认采集器无此信息，需通过 ghw + gopsutil 综合判定 |
| `k8s_node` | 是否为 K8s 节点 | 默认采集器无此信息，需检测 kubelet 进程 |

---

## 2. siliconflow_asset_cpu_device_frequency_mhz

**变更说明：** 移除整个指标，CPU 频率信息由 `cpufreq` 采集器提供。

### 旧指标 → 替代指标映射

| 旧指标 | 旧标签 | 替代指标 | 替代标签 | 采集器 |
|---|---|---|---|---|
| `siliconflow_asset_cpu_device_frequency_mhz` | `uuid`, `socket` | `node_cpu_frequency_min_hertz` | `cpu` | cpufreq |
| | | `node_cpu_frequency_max_hertz` | `cpu` | cpufreq |
| | | `node_cpu_scaling_frequency_min_hertz` | `cpu` | cpufreq |
| | | `node_cpu_scaling_frequency_max_hertz` | `cpu` | cpufreq |
| | | `node_cpu_scaling_frequency_hertz` | `cpu` | cpufreq |

> **注意：** 旧指标报告的是 CPU 基频（静态，来自 /proc/cpuinfo），cpufreq 报告的是运行时频率（动态，来自 sysfs）。`node_cpu_frequency_max_hertz` 通常接近基频。

### 移除的 CPU 总量指标

**变更说明：** 移除机器级 CPU 总量指标（`cpu_sockets` / `cpu_cores` / `cpu_threads`），只保留 per-socket 设备级指标。机器级总量改由消费方从 per-socket 指标派生：`sockets = count(cpu_info 按 socket 去重)`、`cores = sum(cpu_device_cores)`、`threads = sum(cpu_device_threads)`。同时新增 `cpu_device_threads` 补齐此前缺失的 per-socket 线程数。

| 旧指标 | 旧标签 | 派生方式 |
|---|---|---|
| `siliconflow_asset_cpu_sockets` | `uuid` | `count(cpu_info 按 socket 去重)` |
| `siliconflow_asset_cpu_cores` | `uuid` | `sum(cpu_device_cores)` |
| `siliconflow_asset_cpu_threads` | `uuid` | `sum(cpu_device_threads)` |

| 新增指标 | 标签 | 说明 |
|---|---|---|
| `siliconflow_asset_cpu_device_threads` | `uuid`, `socket` | 单插槽逻辑线程数 |

### 保留的 asset_cpu 指标

| 指标 | 标签 | 说明 | 为何保留 |
|---|---|---|---|
| `siliconflow_asset_cpu_info` | `uuid`, `socket`, `model_name`, `vendor_id` | CPU 型号标识 | 默认采集器无此信息 |
| `siliconflow_asset_cpu_device_cores` | `uuid`, `socket` | 单插槽核数 | 默认采集器无此信息 |
| `siliconflow_asset_cpu_device_threads` | `uuid`, `socket` | 单插槽逻辑线程数 | 默认采集器无此信息 |
| `siliconflow_asset_cpu_device_cache_kb` | `uuid`, `socket` | 单插槽缓存大小 | 默认采集器无此信息 |

---

## 3. siliconflow_asset_net_info

**变更说明：** 移除 `mac` 标签，MAC 地址由 `netclass` 采集器的 `node_network_info` 提供。

### 新指标格式

```
siliconflow_asset_net_info{uuid="<uuid>", name="<iface>", physical="<true|false>", master="<bond>", slaves="<s1,s2>", vendor="<pci_vendor>", driver="<driver>"} 1
```

### 旧标签 → 替代指标映射

| 旧标签 | 替代指标 | 替代标签 | 采集器 |
|---|---|---|---|
| `mac` | `node_network_info` | `address` | netclass |

### 保留的标签

| 标签 | 说明 | 为何保留 |
|---|---|---|
| `uuid` | 机器唯一标识 | asset 采集器的关联键 |
| `name` | 网卡名称 | 指标行的主标识符，不可移除 |
| `physical` | 是否物理网卡 | 默认采集器无此信息，需检测 /sys/class/net/\<name\>/device |
| `master` | 从属的 bond 口 | 默认采集器无此信息 |
| `slaves` | bond 下挂从口列表 | 默认采集器无此信息 |
| `vendor` | PCI 厂商 ID | 默认采集器无此信息 |
| `driver` | 驱动名称 | 默认采集器无此信息 |

---

## 4. siliconflow_asset_memory_total_mb

**变更说明：** 移除整个指标，总内存量由 `meminfo` 采集器提供。

### 旧指标 → 替代指标映射

| 旧指标 | 替代指标 | 换算关系 | 采集器 |
|---|---|---|---|
| `siliconflow_asset_memory_total_mb` | `node_memory_MemTotal_bytes` | `旧值 ≈ 新值 / 1024 / 1024` | meminfo |

### 保留的 asset_memory 指标

| 指标 | 标签 | 说明 | 为何保留 |
|---|---|---|---|
| `siliconflow_asset_memory_module_info` | `uuid`, `locator`, `bank_locator`, `size`, `type`, `speed`, `manufacturer`, `serial`, `part_number` | DIMM 模块身份信息 | 默认采集器无此信息，需 dmidecode 采集 |

---

## 5. 无变更的指标

以下指标与默认采集器无重复，保持不变：

| 指标 | 说明 |
|---|---|
| `siliconflow_asset_disk_info` | 磁盘身份信息（name, type, model, vendor, serial），默认 diskstats 无此信息 |
| `siliconflow_asset_disk_size_gb` | 磁盘容量，默认 diskstats 仅报告 I/O 统计 |
| `siliconflow_asset_gpu_info` | GPU/NPU 身份信息（支持 NVIDIA / 华为 NPU / 摩尔线程），默认采集器无 GPU 采集 |

---

## PromQL 迁移示例

```promql
# === machine ===

# 旧：查询厂商
siliconflow_asset_machine_info{vendor="Dell Inc."}
# 新：
node_dmi_info{system_vendor="Dell Inc."}

# 旧：查询操作系统
siliconflow_asset_machine_info{os="ubuntu"}
# 新：
node_os_info{name="ubuntu"}

# 旧：查询内核版本
siliconflow_asset_machine_info{kernel="5.15.0-91-generic"}
# 新：
node_uname_info{release="5.15.0-91-generic"}

# === cpu ===

# 旧：查询 CPU 基频
siliconflow_asset_cpu_device_frequency_mhz{socket="0"}
# 新：查询 CPU 最大频率（通常接近基频）
node_cpu_frequency_max_hertz{cpu="0"}

# === net ===

# 旧：查询网卡 MAC 地址
siliconflow_asset_net_info{mac="aa:bb:cc:dd:ee:ff"}
# 新：
node_network_info{address="aa:bb:cc:dd:ee:ff"}

# 旧：关联网卡 MAC 与 vendor/driver
siliconflow_asset_net_info{vendor="0x15b3"}
# 新：通过 device 名 join
siliconflow_asset_net_info{vendor="0x15b3"} * on(instance, device) group_left(address)
  label_replace(node_network_info, "device", "$1", "device", "(.*)")

# === memory ===

# 旧：查询总内存 (MB)
siliconflow_asset_memory_total_mb
# 新：查询总内存 (bytes)
node_memory_MemTotal_bytes / 1024 / 1024
```
