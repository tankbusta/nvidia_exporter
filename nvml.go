package main

/*
#cgo CFLAGS: -I/usr/local/cuda-8.0/include
#cgo LDFLAGS: -lnvidia-ml -L/usr/lib/nvidia-367

#include "bridge.h"
*/
import "C"
import (
	"errors"
	"fmt"
	"unsafe"
)

var (
	errNoErrorString = errors.New("nvml: expected an error from driver but got nothing")
	errNoError       = errors.New("nvml: getGoError called on a successful API call")
)

func getGoError(result C.nvmlReturn_t) (err error) {
	var errString *C.char

	if result == C.NVML_SUCCESS {
		err = errNoError
		return
	}

	if errString = C.nvmlErrorString(result); err != nil {
		err = errNoErrorString
		return
	}

	err = fmt.Errorf("nvml: %s", C.GoString(errString))
	return
}

// Device describes the NVIDIA GPU device attached to the host
type Device struct {
	DeviceName string
	DeviceUUID string
	d          C.nvmlDevice_t
	i          int
}

// NVMLMemory contains information about the memory allocation of a device
type NVMLMemory struct {
	// Unallocated FB memory (in bytes).
	Free int64
	// Total installed FB memory (in bytes).
	Total int64
	// Allocated FB memory (in bytes). Note that the driver/GPU always sets
	// aside a small amount of memory for bookkeeping.
	Used int64
}

func newDevice(nvmlDevice C.nvmlDevice_t, idx int) (dev Device, err error) {
	dev = Device{
		d: nvmlDevice,
		i: idx,
	}

	if dev.DeviceUUID, err = dev.UUID(); err != nil {
		return
	}

	if dev.DeviceName, err = dev.Name(); err != nil {
		return
	}
	return
}

func (s *Device) callGetTextFunc(f C.getNvmlCharProperty, sz C.uint) (rval string, err error) {
	buf := make([]byte, sz)
	cs := C.CString(string(buf))
	defer C.free(unsafe.Pointer(cs))

	if result := C.bridge_get_text_property(f, s.d, cs, sz); result != C.NVML_SUCCESS {
		err = getGoError(result)
		return
	}

	rval = C.GoString(cs)
	return
}

func (s *Device) callGetIntFunc(f C.getNvmlIntProperty) (rval int, err error) {
	var valC C.uint
	if result := C.bridge_get_int_property(f, s.d, &valC); result != C.NVML_SUCCESS {
		err = getGoError(result)
		return
	}
	rval = int(valC)
	return
}

// UUID returns the Device's Unique ID
func (s *Device) UUID() (uuid string, err error) {
	uuid, err = s.callGetTextFunc(C.getNvmlCharProperty(C.nvmlDeviceGetUUID), C.NVML_DEVICE_UUID_BUFFER_SIZE)
	return
}

// Name returns the Device's Name and is not guaranteed to exceed 64 characters in length
func (s *Device) Name() (name string, err error) {
	name, err = s.callGetTextFunc(C.getNvmlCharProperty(C.nvmlDeviceGetName), C.NVML_DEVICE_NAME_BUFFER_SIZE)
	return
}

// GetUtilization returns the GPU and memory usage returned as a percentage used of a given GPU device
func (s *Device) GetUtilization() (gpu, memory int, err error) {
	var utilRates C.nvmlUtilization_t
	if result := C.nvmlDeviceGetUtilizationRates(s.d, &utilRates); result != C.NVML_SUCCESS {
		err = getGoError(result)
		return
	}
	gpu = int(utilRates.gpu)
	memory = int(utilRates.memory)
	return
}

// GetPowerUsage returns the power consumption of the GPU in watts
func (s *Device) GetPowerUsage() (usage int, err error) {
	usage, err = s.callGetIntFunc(C.getNvmlIntProperty(C.nvmlDeviceGetPowerUsage))
	// nvmlDeviceGetPowerUsage returns milliwatts.. convert to watts
	usage = usage / 1000
	return
}

// GetTemperature returns the Device's temperature in Farenheit and celsius
func (s *Device) GetTemperature() (tempF, tempC int, err error) {
	var tempc C.uint
	if result := C.nvmlDeviceGetTemperature(s.d, C.NVML_TEMPERATURE_GPU, &tempc); result != C.NVML_SUCCESS {
		err = getGoError(result)
		return
	}
	tempC = int(tempc)
	tempF = int(tempC*9/5 + 32)
	return
}

// GetMemoryInfo retrieves the amount of used, free and total memory available on the device, in bytes.
func (s *Device) GetMemoryInfo() (memInfo NVMLMemory, err error) {
	var res C.nvmlMemory_t

	if result := C.nvmlDeviceGetMemoryInfo(s.d, &res); result != C.NVML_SUCCESS {
		err = getGoError(result)
		return
	}

	memInfo.Free = int64(res.free)
	memInfo.Total = int64(res.total)
	memInfo.Used = int64(res.used)
	return
}

// InitNVML initializes NVML
func InitNVML() (err error) {
	if result := C.nvmlInit(); result != C.NVML_SUCCESS {
		err = getGoError(result)
		return
	}
	return
}

// ShutdownNVML all resources that were created when we initialized
func ShutdownNVML() (err error) {
	if result := C.nvmlShutdown(); result != C.NVML_SUCCESS {
		err = getGoError(result)
		return
	}
	return
}

// GetDeviceCount returns the # of CUDA devices present on the host
func GetDeviceCount() (count int, err error) {
	var cnt C.uint

	if result := C.nvmlDeviceGetCount(&cnt); result != C.NVML_SUCCESS {
		err = getGoError(result)
		return
	}
	count = int(cnt)
	return
}

// DeviceGetHandleByIndex acquires the handle for a particular device, based on its index.
func DeviceGetHandleByIndex(idx int) (device *C.nvmlDevice_t, err error) {
	if result := C.nvmlDeviceGetHandleByIndex(C.uint(idx), device); result != C.NVML_SUCCESS {
		err = getGoError(result)
		return
	}
	return
}

// GetDevices returns a list of all installed CUDA devices
func GetDevices() (devices []Device, err error) {
	var nvdev C.nvmlDevice_t

	devCount, err := GetDeviceCount()
	if err != nil {
		return
	}

	devices = make([]Device, devCount)

	for i := 0; i <= devCount-1; i++ {
		if result := C.nvmlDeviceGetHandleByIndex(C.uint(i), &nvdev); result != C.NVML_SUCCESS {
			err = getGoError(result)
			return
		}

		if devices[i], err = newDevice(nvdev, i); err != nil {
			return
		}
	}

	return
}
