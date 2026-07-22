package cmdb

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"github.com/prometheus/node_exporter/collector/asset/cmdb/model"
)

// gpuVendorCollectors maps a canonical vendor name (as produced by
// identifyLspciVendor) to the specialized smi-based collector for that
// vendor. Adding a new GPU vendor to the fleet = add one entry here plus a
// collectXxx function; CollectGPU / probeGpuVendor need no changes.
var gpuVendorCollectors = map[string]func(*model.GPU) bool{
	"nvidia":   collectNVIDIA,
	"huawei":   collectHuaweiNPU,
	"mthreads": collectMThreads,
}

// CollectGPU enumerates the host's GPUs in two stages:
//  1. Run lspci to detect the GPU vendor from PCI display-class (0x03) and
//     processing-accelerator-class (0x12) devices.
//  2. Dispatch the vendor's specialized smi tool (nvidia-smi / npu-smi) to
//     fetch identity + runtime fields. Only if the smi tool reports nothing
//     (cards passed through to guests, driver broken) does the collector fall
//     back to the lspci enumeration (PCI identity only, no runtime metrics).
//
// Vendors without a registered collector (Intel iGPU, AMD, etc.) are dropped.
// The fleet is assumed single-vendor per host, so the first recognized vendor
// drives routing.
func CollectGPU() (*model.GPU, error) {
	return collectGPUCore(runLspciOrEmpty(), gpuVendorCollectors), nil
}

// collectGPUCore is the testable core of CollectGPU (no shell-out). lspciOut is
// the output of `lspci -Dnn`; collectors maps vendor→smi collector.
func collectGPUCore(lspciOut string, collectors map[string]func(*model.GPU) bool) *model.GPU {
	g := &model.GPU{Devices: []model.GPUDevice{}}
	lspciDevs := parseLspciGPU(lspciOut)
	vendor := probeGpuVendor(lspciDevs, collectors)
	fn, ok := collectors[vendor]
	if !ok {
		// Unknown vendor or no GPU/accelerator-class device: nothing to collect.
		return g
	}
	if fn(g) {
		// smi succeeded with ≥1 card: use its richer output (runtime + identity).
		return g
	}
	// smi empty (passthrough / driver broken): fall back to lspci enumeration
	// of the detected vendor's cards. PCI identity only, no runtime fields.
	g.Devices = filterDevs(lspciDevs, vendor)
	return g
}

// runLspciOrEmpty runs `lspci -Dnn` and returns its stdout, or "" on any error
// (lspci not installed, non-zero exit). Returns "" rather than propagating the
// error so CollectGPU degrades gracefully to an empty GPU set.
//
// -D: always print the PCI domain (so bus IDs are unique across hosts).
// -nn: print both textual names and numeric vendor:device IDs — needed to
//
//	identify brand-new SKUs whose PCI ID isn't in pci.ids yet (e.g. the
//	0x2b85 RTX 5090 shows up as "Device 2b85" without it).
func runLspciOrEmpty() string {
	if !commandExists("lspci") {
		return ""
	}
	out, err := runCmd("lspci", "-Dnn")
	if err != nil {
		return ""
	}
	return out
}

// probeGpuVendor returns the canonical vendor of the first display-class PCI
// device that has a registered collector, or "" if none match. Used to route
// CollectGPU to the right smi tool.
func probeGpuVendor(devs []model.GPUDevice, collectors map[string]func(*model.GPU) bool) string {
	for _, d := range devs {
		if _, ok := collectors[d.Vendor]; ok {
			return d.Vendor
		}
	}
	return ""
}

// filterDevs returns the subset of devs belonging to vendor, with Index
// renumbered to a contiguous 0..N-1 so downstream consumers (the store's
// change-diff, which keys GPU devices by Index) see a stable sequence.
func filterDevs(devs []model.GPUDevice, vendor string) []model.GPUDevice {
	out := make([]model.GPUDevice, 0, len(devs))
	for _, d := range devs {
		if d.Vendor == vendor {
			out = append(out, d)
		}
	}
	for i := range out {
		out[i].Index = i
	}
	return out
}

func collectNVIDIA(g *model.GPU) bool {
	if !commandExists("nvidia-smi") {
		return false
	}
	out, err := runCmd("nvidia-smi",
		"--query-gpu=index,name,uuid,serial,vbios_version,memory.total,memory.used,memory.free,utilization.gpu,temperature.gpu,power.draw,driver_version",
		"--format=csv,noheader,nounits")
	if err != nil {
		return false
	}
	devs := parseNVIDIA(out)
	if len(devs) == 0 {
		return false
	}
	g.Devices = append(g.Devices, devs...)
	return true
}

func parseNVIDIA(out string) []model.GPUDevice {
	var devices []model.GPUDevice
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Split(line, ",")
		if len(fields) < 12 {
			continue
		}
		for i := range fields {
			fields[i] = strings.TrimSpace(fields[i])
		}
		devices = append(devices, model.GPUDevice{
			Index:           atoiSafe(fields[0]),
			Vendor:          "nvidia",
			Name:            fields[1],
			UUID:            fields[2],
			Serial:          fields[3],
			FirmwareVersion: fields[4],
			MemoryTotalMB:   atouSafe(fields[5]),
			MemoryUsedMB:    atouSafe(fields[6]),
			MemoryFreeMB:    atouSafe(fields[7]),
			Utilization:     atofSafe(fields[8]),
			Temperature:     atofSafe(fields[9]),
			PowerW:          atofSafe(fields[10]),
			DriverVersion:   fields[11],
			Health:          "OK",
			RuntimeMetrics:  true,
		})
	}
	return devices
}

var (
	npuVerRe    = regexp.MustCompile(`Version:\s*(\S+)`)
	npuMemRe    = regexp.MustCompile(`(\d+)\s*/\s*(\d+)`)
	npuBusIDRe  = regexp.MustCompile(`^[0-9A-Fa-f]{4}:[0-9A-Fa-f]{2}:\d{2}\.\d`)
	npuSerialRe = regexp.MustCompile(`(?im)^\s*Serial\s*Number\s*:\s*(\S+)`)
	npuFWRe     = regexp.MustCompile(`(?im)^\s*Firmware\s*Version\s*:\s*(\S+)`)
	pciIDRe     = regexp.MustCompile(`\[([0-9A-Fa-f]{4}:[0-9A-Fa-f]{4})\]`)
)

func collectHuaweiNPU(g *model.GPU) bool {
	if !commandExists("npu-smi") {
		return false
	}
	out, err := runCmd("npu-smi", "info")
	if err != nil {
		return false
	}
	devices := parseHuaweiNPU(out)
	for i := range devices {
		boardOut, err := runCmd("npu-smi", "info", "-t", "board", "-i", strconv.Itoa(devices[i].Index))
		if err != nil {
			continue
		}
		if m := npuSerialRe.FindStringSubmatch(boardOut); m != nil {
			devices[i].Serial = strings.TrimSpace(m[1])
		}
		if m := npuFWRe.FindStringSubmatch(boardOut); m != nil {
			devices[i].FirmwareVersion = strings.TrimSpace(m[1])
		}
	}
	if len(devices) == 0 {
		return false
	}
	g.Devices = append(g.Devices, devices...)
	return true
}

func parseHuaweiNPU(out string) []model.GPUDevice {
	var driverVer string
	if m := npuVerRe.FindStringSubmatch(out); m != nil {
		driverVer = m[1]
	}

	var devices []model.GPUDevice
	var cur *model.GPUDevice
	// afterSep is true after a "+---+ / +===+" separator: only the first
	// data line following one is a candidate card header. A single NPU card
	// may span several separator-bounded blocks — e.g. 310P3 repeats the
	// card header once per chip, with "+---+" between chips. We therefore do
	// NOT flush on every separator; we keep merging into the current card as
	// long as the NPU index (first column) is unchanged, and only start a
	// new card when the NPU index changes.
	afterSep := false
	flush := func() {
		if cur != nil {
			cur.RuntimeMetrics = true
			devices = append(devices, *cur)
			cur = nil
		}
	}

	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		// The trailing "Process info" section lists one row per running
		// process: "| NPU  Chip | Process id | Process name | Process
		// memory |". Those rows have the same shape as card headers
		// (numeric NPU idx + chip in col1, numeric Process id in the
		// Health column), so without an explicit guard the state machine
		// mints one phantom device per running process whose Name is the
		// chip number and Health is the process id. Stop as soon as we
		// cross into that section, detected by its column header.
		if strings.Contains(line, "Process id") {
			break
		}
		if strings.HasPrefix(trimmed, "+") {
			afterSep = true
			continue
		}
		if !strings.Contains(line, "|") {
			continue
		}
		cols := splitPipe(line)
		if len(cols) < 3 {
			continue
		}
		col1 := strings.Fields(cols[0])
		if len(col1) == 0 {
			continue
		}
		npuIdx, err := strconv.Atoi(col1[0])
		if err != nil {
			continue
		}

		isHeader := afterSep && len(col1) >= 2
		afterSep = false

		if isHeader {
			if cur != nil && npuIdx == cur.Index {
				// Another block of the same NPU (e.g. a per-chip header on
				// 310P3). Keep merging into the existing card; name/health/
				// power/temp come from the first header and are not reset.
				continue
			}
			flush()
			cur = &model.GPUDevice{
				Index:         npuIdx,
				Vendor:        "huawei",
				Name:          col1[1],
				Health:        strings.TrimSpace(cols[1]),
				DriverVersion: driverVer,
			}
			col3 := strings.Fields(cols[2])
			if len(col3) >= 1 {
				cur.PowerW = atofSafe(col3[0])
			}
			if len(col3) >= 2 {
				cur.Temperature = atofSafe(col3[1])
			}
			continue
		}

		// Non-header line within the current card: only chip detail rows
		// whose middle column is a Bus-Id carry useful metrics. Extra rows
		// (alarm-event rows, chip-id rows, etc.) are ignored so their
		// placeholder "0 / 0" values won't clobber real metrics. Memory is
		// summed across chips so a multi-chip card reports its aggregate.
		if cur == nil {
			continue
		}
		busField := strings.TrimSpace(cols[1])
		if !npuBusIDRe.MatchString(busField) {
			continue
		}
		cur.UUID = busField
		col3Fields := strings.Fields(cols[2])
		if len(col3Fields) > 0 {
			cur.Utilization = atofSafe(col3Fields[0])
		}
		matches := npuMemRe.FindAllStringSubmatch(cols[2], -1)
		if len(matches) > 0 {
			last := matches[len(matches)-1]
			cur.MemoryUsedMB += atouSafe(last[1])
			cur.MemoryTotalMB += atouSafe(last[2])
		}
	}
	flush()
	for i := range devices {
		if devices[i].MemoryTotalMB > devices[i].MemoryUsedMB {
			devices[i].MemoryFreeMB = devices[i].MemoryTotalMB - devices[i].MemoryUsedMB
		}
	}
	return devices
}

// collectMThreads enumerates Moore Threads GPUs via `mthreads-gmi --query --json`.
// Returns false (→ lspci fallback) when the tool is absent, fails, or reports no
// cards (passthrough / driver broken). The JSON output is richer than lspci
// (driver version, MTBios, memory, utilization, temperature, power), so on
// success its devices supersede the lspci enumeration.
func collectMThreads(g *model.GPU) bool {
	if !commandExists("mthreads-gmi") {
		return false
	}
	out, err := runCmd("mthreads-gmi", "--query", "--json")
	if err != nil {
		return false
	}
	devs := parseMThreads(out)
	if len(devs) == 0 {
		return false
	}
	g.Devices = append(g.Devices, devs...)
	return true
}

// mthreadsGMI is the top-level shape of `mthreads-gmi --query --json`.
type mthreadsGMI struct {
	DriverVersion string        `json:"Driver Version"`
	GPUs          []mthreadsGPU `json:"GPU"`
}

// mthreadsGPU is one entry of the "GPU" array. PowerReadings is decoded into a
// map rather than a struct because mthreads-gmi emits the power-draw field key
// with a trailing space ("Power Draw "), which is fragile to match verbatim;
// mthreadsMapTrimmed resolves the lookup by trimmed-equals comparison.
type mthreadsGPU struct {
	Index         string            `json:"Index"`
	ProductName   string            `json:"Product Name"`
	GPUUUID       string            `json:"GPU UUID"`
	SerialNumber  string            `json:"Serial Number"`
	MTBiosVersion string            `json:"MTBios Version"`
	FBMemoryUsage mthreadsMem       `json:"FB Memory Usage"`
	Utilization   mthreadsUtil      `json:"Utilization"`
	Temperature   mthreadsTemp      `json:"Temperature"`
	PowerReadings map[string]string `json:"Power Readings"`
}

type mthreadsMem struct {
	Total string `json:"Total"`
	Used  string `json:"Used"`
	Free  string `json:"Free"`
}

type mthreadsUtil struct {
	Gpu    string `json:"Gpu"`
	Memory string `json:"Memory"`
}

type mthreadsTemp struct {
	CurrentTemp string `json:"GPU Current Temp"`
}

// parseMThreads decodes `mthreads-gmi --query --json` output into GPUDevice
// records. Every known static + runtime field is populated and RuntimeMetrics
// is set true (the smi run succeeded), mirroring the NVIDIA/Huawei collectors.
// On any decode error it returns nil so the caller falls back to lspci.
func parseMThreads(out string) []model.GPUDevice {
	var doc mthreadsGMI
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		return nil
	}
	devs := make([]model.GPUDevice, 0, len(doc.GPUs))
	for _, g := range doc.GPUs {
		powerDraw := mthreadsMapTrimmed(g.PowerReadings, "Power Draw")
		mem := g.FBMemoryUsage
		devs = append(devs, model.GPUDevice{
			Index:           atoiSafe(g.Index),
			Vendor:          "mthreads",
			Name:            g.ProductName,
			UUID:            g.GPUUUID,
			Serial:          g.SerialNumber,
			DriverVersion:   doc.DriverVersion,
			FirmwareVersion: g.MTBiosVersion,
			MemoryTotalMB:   atouSafe(stripMThreadsUnit(mem.Total, "MiB")),
			MemoryUsedMB:    atouSafe(stripMThreadsUnit(mem.Used, "MiB")),
			MemoryFreeMB:    atouSafe(stripMThreadsUnit(mem.Free, "MiB")),
			Utilization:     atofSafe(stripMThreadsUnit(g.Utilization.Gpu, "%")),
			Temperature:     atofSafe(stripMThreadsUnit(g.Temperature.CurrentTemp, "C")),
			PowerW:          atofSafe(stripMThreadsUnit(powerDraw, "W")),
			Health:          "OK",
			RuntimeMetrics:  true,
		})
	}
	return devs
}

// mthreadsMapTrimmed looks up key in m, comparing after trimming whitespace on
// both sides. Used for mthreads-gmi JSON keys that carry irregular whitespace
// (e.g. "Power Draw " has a trailing space) so the caller never has to match
// the exact spacing.
func mthreadsMapTrimmed(m map[string]string, key string) string {
	key = strings.TrimSpace(key)
	for k, v := range m {
		if strings.TrimSpace(k) == key {
			return v
		}
	}
	return ""
}

// stripMThreadsUnit removes a trailing unit suffix (e.g. "MiB", "%", "C", "W")
// from an mthreads-gmi value string and returns the bare number. Values without
// the suffix (e.g. "N/A") survive unchanged so atoi/atof safely yield 0.
func stripMThreadsUnit(s, suffix string) string {
	s = strings.TrimSpace(s)
	return strings.TrimSpace(strings.TrimSuffix(s, suffix))
}

// parseLspciGPU parses `lspci -Dnn` output and returns one GPUDevice per PCI
// display-class (VGA / 3D / Display / XGA controller) or processing-accelerator
// ("Processing accelerators") function, for ANY vendor — not just NVIDIA. The
// audio subfunction paired with most consumer GPUs is skipped so each card is
// counted once via its display function. Huawei Ascend NPUs expose the
// "Processing accelerators" class (0x12) rather than a display class, which is
// why both classes are matched here.
//
// Example input lines:
//
//	0000:16:00.0 VGA compatible controller [0300]: NVIDIA Corporation Device 2b85 [10de:2b85] (rev ff)
//	00:02.0 VGA compatible controller [0300]: Intel Corporation CoffeeLake-S GT2 [UHD Graphics 630] [8086:3e98] (rev 02)
//	0000:43:00.0 3D controller [0302]: NVIDIA Corporation GA100 [A100 SXM4 40GB] [10de:20b5] (rev a1)
//	0000:18:00.0 Processing accelerators [1200]: Huawei Technologies Co., Ltd. Device d802 [19e5:d802] (rev 20)
//
// Only PCI-level fields are populated: memory/utilization/temperature/power/
// driver/firmware require a host-bound driver and are left empty. Vendor is
// derived from the numeric PCI vendor:device ID when available (preferred,
// requires `lspci -nn`), with a text-based fallback for plain `lspci` output.
func parseLspciGPU(out string) []model.GPUDevice {
	var devices []model.GPUDevice
	idx := 0
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !isGpuOrAccelerator(line) {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		bus := fields[0]
		// lspci without -D omits the domain; normalize to canonical PCI BDF
		// (dom:bus:dev.fn) to align with nvidia-smi / npu-smi bus IDs.
		if strings.Count(bus, ":") == 1 {
			bus = "0000:" + bus
		}

		name, pciID := parseLspciDeviceDesc(line)
		devices = append(devices, model.GPUDevice{
			Index:  idx,
			Vendor: identifyLspciVendor(pciID, name),
			Name:   name,
			UUID:   bus,
			Health: "Unknown",
			Serial: pciID,
		})
		idx++
	}
	return devices
}

// isGpuOrAccelerator reports whether an lspci line describes a PCI device of
// interest to GPU collection: display controllers (base class 0x03) or
// processing accelerators (base class 0x12).
//
// Display subclasses (0x03): "VGA compatible controller" (0x0300, used by
// consumer/RTX cards and the ASPEED BMC VGA), "XGA compatible controller"
// (0x0301), "3D controller" (0x0302, compute-only cards like A100/H100),
// "Display controller" (0x0380).
//
// Processing accelerators (0x12): "Processing accelerators" — the class Huawei
// Ascend NPUs (910B2C et al.) expose; they are NOT display controllers, so
// without this branch the huawei routing path would silently drop every NPU
// host.
func isGpuOrAccelerator(line string) bool {
	return strings.Contains(line, "VGA compatible controller") ||
		strings.Contains(line, "XGA compatible controller") ||
		strings.Contains(line, "3D controller") ||
		strings.Contains(line, "Display controller") ||
		strings.Contains(line, "Processing accelerators")
}

// pciVendorMap maps well-known PCI vendor IDs (lowercase 4-digit hex) to the
// canonical lowercase vendor name used by the collector. Source:
// https://pci-ids.ucw.cz/ — extend as new vendors appear in the fleet.
var pciVendorMap = map[string]string{
	"10de": "nvidia",   // NVIDIA Corporation
	"1002": "amd",      // Advanced Micro Devices, Inc.
	"8086": "intel",    // Intel Corporation
	"19e5": "huawei",   // Huawei Technologies Co., Ltd.
	"1ed5": "mthreads", // Moore Threads Technology Co.,Ltd
}

// identifyLspciVendor resolves the canonical vendor name from the numeric PCI
// vendor:device ID (preferred, available with `lspci -nn`) and falls back to
// substring matching on the textual description (plain `lspci` output). When
// neither yields a known vendor the lowercase 4-digit PCI vendor ID is
// returned (e.g. "1cee") so the device stays identifiable downstream; if even
// that is unavailable it returns "unknown".
func identifyLspciVendor(pciID, name string) string {
	if len(pciID) >= 4 {
		if canonical, ok := pciVendorMap[strings.ToLower(pciID[:4])]; ok {
			return canonical
		}
	}
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "nvidia"):
		return "nvidia"
	case strings.Contains(lower, "huawei"), strings.Contains(lower, "ascend"):
		return "huawei"
	case strings.Contains(lower, "advanced micro devices"), strings.Contains(lower, "amd"):
		return "amd"
	case strings.Contains(lower, "intel"):
		return "intel"
	case strings.Contains(lower, "moore threads"):
		return "mthreads"
	}
	if len(pciID) >= 4 {
		return strings.ToLower(pciID[:4])
	}
	return "unknown"
}

// parseLspciDeviceDesc extracts the textual device description and the numeric
// PCI vendor:device ID from a single lspci line. The line may carry numeric
// IDs from -nn (preferred) or be plain lspci output.
func parseLspciDeviceDesc(line string) (name, pciID string) {
	// Strip the leading "<bus> <class> [classcode]: " (with -nn) or
	// "<bus> <class>: " (plain lspci) prefix to get the vendor + device text.
	rest := line
	if i := strings.Index(rest, "]: "); i >= 0 {
		rest = rest[i+3:]
	} else if i := strings.Index(rest, ": "); i >= 0 {
		rest = rest[i+2:]
	} else {
		return "", ""
	}
	// The vendor:device bracket (e.g. "[10de:2b85]") is the LAST "[hhhh:hhhh]"
	// on the line: the class-code bracket was stripped above together with
	// the class name. Cut it (and anything after, such as "(rev X)") off the
	// name and capture the PCI ID.
	if i := strings.LastIndex(rest, " ["); i >= 0 {
		tail := rest[i+1:]
		rest = rest[:i]
		if m := pciIDRe.FindStringSubmatch(tail); m != nil {
			pciID = m[1]
		}
	}
	// A trailing "(rev X)" can survive in plain lspci output (no -nn) or when
	// the vendor:device bracket is absent because lspci doesn't know the IDs.
	if i := strings.LastIndex(rest, "(rev"); i >= 0 {
		rest = rest[:i]
	}
	name = strings.TrimSpace(rest)
	return name, pciID
}

func splitPipe(line string) []string {
	parts := strings.Split(line, "|")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func atoiSafe(s string) int {
	v, _ := strconv.Atoi(strings.TrimSpace(s))
	return v
}

func atouSafe(s string) uint64 {
	v, _ := strconv.ParseUint(strings.TrimSpace(s), 10, 64)
	return v
}

func atofSafe(s string) float64 {
	v, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return v
}
