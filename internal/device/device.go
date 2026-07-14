// Package device loads the fleet's device roster from CSV.
package device

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
)

// Device is a single piece of fleet equipment known to the API.
type Device struct {
	ID string
}

// LoadCSV reads a devices.csv file (a single "device_id" column, with
// header) and returns the devices it describes.
func LoadCSV(path string) ([]Device, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open devices csv: %w", err)
	}
	defer f.Close()

	r := csv.NewReader(f)

	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("read devices csv header: %w", err)
	}
	idCol := -1
	for i, col := range header {
		if col == "device_id" {
			idCol = i
			break
		}
	}
	if idCol == -1 {
		return nil, fmt.Errorf("devices csv missing required %q column", "device_id")
	}

	var devices []Device
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read devices csv row: %w", err)
		}
		devices = append(devices, Device{ID: record[idCol]})
	}

	return devices, nil
}
