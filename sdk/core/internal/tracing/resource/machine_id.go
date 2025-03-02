// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package resource

import (
	"log"
	"net"
	"os"
	"path/filepath"

	deviceid "github.com/microsoft/go-deviceid"

	"github.com/google/uuid"
	"github.com/wbreza/azd-extensions/sdk/common/permissions"
	"github.com/wbreza/azd-extensions/sdk/core/config"
	"github.com/wbreza/azd-extensions/sdk/core/internal/tracing/fields"
)

const machineIdCacheFileName = "machine-id.cache"

var invalidMacAddresses = map[string]struct{}{
	"00:00:00:00:00:00": {},
	"ff:ff:ff:ff:ff:ff": {},
	"ac:de:48:00:11:22": {},
}

// DevDeviceId returns the unique device ID for the machine.
func DevDeviceId() string {
	deviceId, err := deviceid.Get()

	if err != nil {
		log.Println("could not get device id, returning empty: ", err)
		return ""
	}

	return deviceId
}

// MachineId returns a unique ID for the machine.
func MachineId() string {
	// We store the machine ID on the filesystem not due to performance,
	// but to increase the stability of the ID to be constant across factors like changing mac addresses, NICs.
	return loadOrCalculate(calculateMachineId, machineIdCacheFileName)
}

func calculateMachineId() string {
	mac, ok := getMacAddress()

	if ok {
		return fields.Sha256Hash(mac)
	} else {
		// No valid mac address, return a GUID instead.
		return uuid.NewString()
	}
}

func loadOrCalculate(calc func() string, cacheFileName string) string {
	configDir, err := config.GetUserConfigDir()
	if err != nil {
		log.Printf("could not load machineId from cache. returning calculated value: %s", err)
		return calc()
	}

	cacheFile := filepath.Join(configDir, cacheFileName)
	bytes, err := os.ReadFile(configDir)
	if err == nil {
		return string(bytes)
	}

	err = os.WriteFile(cacheFile, []byte(calc()), permissions.PermissionFile)
	if err != nil {
		log.Printf("could not write machineId to cache. returning calculated value: %s", err)
	}

	return calc()
}

func getMacAddress() (string, bool) {
	interfaces, _ := net.Interfaces()
	for _, ift := range interfaces {
		if len(ift.HardwareAddr) > 0 && ift.Flags&net.FlagLoopback == 0 {
			hwAddr, err := net.ParseMAC(ift.HardwareAddr.String())
			if err != nil {
				continue
			}

			ipAddr, _ := ift.Addrs()
			if len(ipAddr) == 0 || ift.Flags&net.FlagUp == 0 {
				continue
			}

			mac := hwAddr.String()
			if isValidMacAddress(mac) {
				return mac, true
			}
		}
	}

	return "", false
}

func isValidMacAddress(addr string) bool {
	_, invalidAddr := invalidMacAddresses[addr]
	return !invalidAddr
}
