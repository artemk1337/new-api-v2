package main

import (
	"context"
	"log"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/disk"
	"github.com/shirou/gopsutil/load"
	"github.com/shirou/gopsutil/mem"
	"github.com/shirou/gopsutil/process"
)

const (
	sampleInterval    = 10 * time.Second
	cleanupInterval   = time.Hour
	topProcessLimit   = 3
	maxPendingSamples = int(cleanupInterval / sampleInterval)
)

type processSnapshot struct {
	PID      int
	Name     string
	CPUTime  float64
	RSSBytes uint64
}

type topProcess struct {
	PID      int     `json:"pid"`
	Name     string  `json:"name"`
	CPUUsage float64 `json:"cpu_usage"`
	RSSBytes uint64  `json:"rss_bytes"`
}

type collector struct {
	mu           sync.Mutex
	previous     map[int]processSnapshot
	lastAt       time.Time
	lastCPUTimes cpu.TimesStat
	hasCPUTimes  bool
	pending      []*model.SystemTelemetrySample
}

func main() {
	common.InitEnv()
	if err := model.InitDB(); err != nil {
		log.Fatal(err)
	}
	defer func() { _ = model.CloseDB() }()

	collector := collector{previous: make(map[int]processSnapshot)}
	collectAndStore(&collector)
	ticker := time.NewTicker(sampleInterval)
	cleanupTicker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()
	defer cleanupTicker.Stop()
	for {
		select {
		case <-ticker.C:
			collectAndStore(&collector)
		case <-cleanupTicker.C:
			if err := model.DeleteSystemTelemetrySamplesBefore(time.Now().Add(-time.Duration(model.SystemTelemetryRetentionSeconds) * time.Second).Unix()); err != nil {
				log.Printf("cleanup telemetry samples: %v", err)
			}
		}
	}
}

func collectAndStore(collector *collector) {
	sample, err := collector.collect(context.Background())
	if err != nil {
		log.Printf("collect telemetry: %v", err)
		return
	}
	collector.store(sample)
}

func (collector *collector) store(sample *model.SystemTelemetrySample) {
	for len(collector.pending) > 0 {
		if err := model.CreateSystemTelemetrySample(collector.pending[0]); err != nil {
			log.Printf("store pending telemetry: %v", err)
			break
		}
		collector.pending = collector.pending[1:]
	}
	if err := model.CreateSystemTelemetrySample(sample); err == nil {
		return
	} else {
		log.Printf("store telemetry: %v", err)
	}
	collector.pending = append(collector.pending, sample)
	if len(collector.pending) > maxPendingSamples {
		collector.pending = collector.pending[len(collector.pending)-maxPendingSamples:]
	}
}

func (collector *collector) collect(ctx context.Context) (*model.SystemTelemetrySample, error) {
	collector.mu.Lock()
	defer collector.mu.Unlock()

	now := time.Now()
	virtualMemory, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return nil, err
	}
	swapMemory, err := mem.SwapMemoryWithContext(ctx)
	if err != nil {
		return nil, err
	}
	cpuPercent, err := cpu.PercentWithContext(ctx, 0, false)
	if err != nil {
		return nil, err
	}
	cpuTimes, err := cpu.TimesWithContext(ctx, false)
	if err != nil {
		return nil, err
	}
	loadAverage, err := load.AvgWithContext(ctx)
	if err != nil {
		return nil, err
	}
	diskPath := os.Getenv("SYSTEM_TELEMETRY_DISK_PATH")
	if diskPath == "" {
		diskPath = "/"
	}
	diskUsage, err := disk.UsageWithContext(ctx, diskPath)
	if err != nil {
		return nil, err
	}

	usage := 0.0
	if len(cpuPercent) > 0 {
		usage = cpuPercent[0]
	}
	iowait := 0.0
	if len(cpuTimes) > 0 {
		if collector.hasCPUTimes {
			iowait = ioWaitPercent(collector.lastCPUTimes, cpuTimes[0])
		}
		collector.lastCPUTimes = cpuTimes[0]
		collector.hasCPUTimes = true
	}
	top := collector.topProcesses(ctx, now)
	topJSON, err := common.Marshal(top)
	if err != nil {
		return nil, err
	}
	return &model.SystemTelemetrySample{
		NodeName:      common.NodeName,
		CollectedAt:   now.Unix(),
		CPUUsage:      usage,
		MemoryUsage:   virtualMemory.UsedPercent,
		SwapUsedBytes: swapMemory.Used,
		SwapUsage:     swapMemory.UsedPercent,
		IOWait:        iowait,
		LoadAverage1:  loadAverage.Load1,
		DiskUsage:     diskUsage.UsedPercent,
		TopProcesses:  string(topJSON),
	}, nil
}

func ioWaitPercent(previous cpu.TimesStat, current cpu.TimesStat) float64 {
	deltaTotal := current.Total() - previous.Total()
	deltaIOWait := current.Iowait - previous.Iowait
	if deltaTotal <= 0 || deltaIOWait < 0 {
		return 0
	}
	return deltaIOWait / deltaTotal * 100
}

func (collector *collector) topProcesses(ctx context.Context, now time.Time) []topProcess {
	pids, err := process.PidsWithContext(ctx)
	if err != nil {
		return topCPUProcesses(nil)
	}
	current := make(map[int]processSnapshot, len(pids))
	duration := now.Sub(collector.lastAt).Seconds()
	leaders := make([]topProcess, 0, len(pids))
	for _, pid := range pids {
		item, err := process.NewProcessWithContext(ctx, pid)
		if err != nil {
			continue
		}
		times, err := item.TimesWithContext(ctx)
		if err != nil {
			continue
		}
		name, err := item.NameWithContext(ctx)
		if err != nil {
			continue
		}
		memoryInfo, err := item.MemoryInfoWithContext(ctx)
		if err != nil {
			continue
		}
		processID := int(pid)
		snapshot := processSnapshot{PID: processID, Name: name, CPUTime: times.User + times.System, RSSBytes: memoryInfo.RSS}
		current[processID] = snapshot
		previous, ok := collector.previous[processID]
		if !ok || duration <= 0 || snapshot.CPUTime < previous.CPUTime {
			continue
		}
		leaders = append(leaders, topProcess{PID: processID, Name: name, CPUUsage: (snapshot.CPUTime - previous.CPUTime) / duration * 100, RSSBytes: memoryInfo.RSS})
	}
	collector.previous = current
	collector.lastAt = now
	return topCPUProcesses(leaders)
}

func topCPUProcesses(processes []topProcess) []topProcess {
	if len(processes) == 0 {
		return []topProcess{}
	}
	sort.Slice(processes, func(i, j int) bool { return processes[i].CPUUsage > processes[j].CPUUsage })
	if len(processes) > topProcessLimit {
		return processes[:topProcessLimit]
	}
	return processes
}
