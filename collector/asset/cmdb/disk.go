package cmdb

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/prometheus/node_exporter/collector/asset/cmdb/model"
)

type lsblkDevice struct {
	Name     string   `json:"name"`
	Model    string   `json:"model"`
	Vendor   string   `json:"vendor"`
	Serial   string   `json:"serial"`
	Size     flexUint `json:"size"`
	Type     string   `json:"type"`
	Children []lsblkDevice `json:"children,omitempty"`
}

type flexUint uint64

func (f *flexUint) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		return nil
	}
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return err
	}
	*f = flexUint(v)
	return nil
}

type lsblkOutput struct {
	BlockDevices []lsblkDevice `json:"blockdevices"`
}

func CollectDisk() (*model.Disk, error) {
	d := &model.Disk{Devices: []model.DiskDevice{}}

	collectLSBLK(d)

	return d, nil
}

func collectLSBLK(d *model.Disk) {
	if !commandExists("lsblk") {
		return
	}

	out, err := runCmd("lsblk", "-b", "-J",
		"-o", "NAME,MODEL,VENDOR,SERIAL,SIZE,TYPE")
	if err != nil {
		return
	}

	var parsed lsblkOutput
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		return
	}

	for _, dev := range parsed.BlockDevices {
		walkLSBLK(dev, d)
	}
}

func walkLSBLK(dev lsblkDevice, d *model.Disk) {
	if uint64(dev.Size) == 0 {
		return
	}
	if dev.Type != "disk" {
		return
	}
	d.Devices = append(d.Devices, model.DiskDevice{
		Name:      dev.Name,
		Type:      dev.Type,
		Model:     dev.Model,
		Vendor:    dev.Vendor,
		Serial:    dev.Serial,
		SizeBytes: uint64(dev.Size),
	})
}
