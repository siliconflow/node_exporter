package model

type Machine struct {
	Type    string `json:"type,omitempty"`
	K8sNode bool   `json:"k8s_node,omitempty"`
}

// CPU carries only per-socket devices. Aggregate counts (sockets/cores/threads)
// are intentionally NOT stored here: they are derived by consumers from the
// device list (sockets = len(Devices), cores/threads = SUM over devices). The
// exporter no longer emits machine-level cpu_sockets/cpu_cores/cpu_threads
// metrics.
type CPU struct {
	Devices []CPUDevice `json:"devices,omitempty"`
}

type CPUDevice struct {
	ModelName string  `json:"model_name,omitempty"`
	VendorID  string  `json:"vendor_id,omitempty"`
	Cores     int     `json:"cores"`
	Threads   int     `json:"threads"`
	Mhz       float64 `json:"mhz,omitempty"`
	CacheKB   int     `json:"cache_kb,omitempty"`
}

type MemoryModule struct {
	Locator      string `json:"locator,omitempty"`
	BankLocator  string `json:"bank_locator,omitempty"`
	Size         string `json:"size,omitempty"`
	Type         string `json:"type,omitempty"`
	Speed        string `json:"speed,omitempty"`
	Manufacturer string `json:"manufacturer,omitempty"`
	Serial       string `json:"serial,omitempty"`
	PartNumber   string `json:"part_number,omitempty"`
}

type Memory struct {
	Modules []MemoryModule `json:"modules,omitempty"`
}

type DiskDevice struct {
	Name      string `json:"name,omitempty"`
	Type      string `json:"type,omitempty"`
	Model     string `json:"model,omitempty"`
	Vendor    string `json:"vendor,omitempty"`
	Serial    string `json:"serial,omitempty"`
	SizeBytes uint64 `json:"size_bytes,omitempty"`
}

type Disk struct {
	Devices []DiskDevice `json:"devices,omitempty"`
}

type GPUDevice struct {
	Index           int     `json:"index"`
	Vendor          string  `json:"vendor,omitempty"`
	Name            string  `json:"name,omitempty"`
	Serial          string  `json:"serial,omitempty"`
	UUID            string  `json:"uuid,omitempty"`
	Health          string  `json:"health,omitempty"`
	MemoryTotalMB   uint64  `json:"memory_total_mb,omitempty"`
	MemoryUsedMB    uint64  `json:"memory_used_mb,omitempty"`
	MemoryFreeMB    uint64  `json:"memory_free_mb,omitempty"`
	Utilization     float64 `json:"utilization,omitempty"`
	Temperature     float64 `json:"temperature,omitempty"`
	PowerW          float64 `json:"power_w,omitempty"`
	DriverVersion   string  `json:"driver_version,omitempty"`
	FirmwareVersion string  `json:"firmware_version,omitempty"`
	// RuntimeMetrics reports whether memory/utilization/temperature/power
	// were actually read from the vendor tool. False means the device was
	// only enumerated via lspci (e.g. passthrough/vfio) and the runtime
	// fields are zero values — emitting them as 0 would be misleading.
	RuntimeMetrics bool `json:"runtime_metrics,omitempty"`
}

type GPU struct {
	Devices []GPUDevice `json:"devices,omitempty"`
}

type NetDevice struct {
	Name     string   `json:"name,omitempty"`
	Physical bool     `json:"physical"`
	Master   string   `json:"master,omitempty"`
	Slaves   []string `json:"slaves,omitempty"`
	Vendor   string   `json:"vendor,omitempty"`
	Driver   string   `json:"driver,omitempty"`
}

type Net struct {
	Devices []NetDevice `json:"devices,omitempty"`
}
