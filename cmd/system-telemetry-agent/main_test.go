package main

import (
	"testing"

	"github.com/shirou/gopsutil/cpu"
	"github.com/stretchr/testify/assert"
)

func TestTopCPUProcessesReturnsThreeLargestValues(t *testing.T) {
	processes := []topProcess{
		{PID: 1, CPUUsage: 10},
		{PID: 2, CPUUsage: 40},
		{PID: 3, CPUUsage: 20},
		{PID: 4, CPUUsage: 30},
	}

	leaders := topCPUProcesses(processes)

	assert.Equal(t, []int{2, 4, 3}, []int{leaders[0].PID, leaders[1].PID, leaders[2].PID})
}

func TestTopCPUProcessesReturnsEmptySlice(t *testing.T) {
	leaders := topCPUProcesses(nil)

	assert.NotNil(t, leaders)
	assert.Empty(t, leaders)
}

func TestIOWaitPercentUsesTotalCPUTime(t *testing.T) {
	previous := cpu.TimesStat{User: 10, System: 10, Idle: 20, Iowait: 0}
	current := cpu.TimesStat{User: 15, System: 15, Idle: 50, Iowait: 10}

	assert.Equal(t, 20.0, ioWaitPercent(previous, current))
}
