package main

import (
	"errors"
	"math"
	"sort"
)

type runRecord struct {
	Variant           string
	Phase             string
	Repeat            int
	Order             int
	WallSeconds       float64
	UserSeconds       float64
	SystemSeconds     float64
	MaxRSSKB          int64
	ExitCode          int
	OutputBytes       int64
	BrunRunID         string
	ResourceBackend   string
	BrunDurationMs    int64
	CgroupCPUUserMs   int64
	CgroupCPUSystemMs int64
	MemoryPeakBytes   int64
	IOReadBytes       int64
	IOWriteBytes      int64
	OOMKillCount      int64
	PIDsPeak          int64
}

type summaryRecord struct {
	Variant              string
	Runs                 int
	MeanWallSeconds      float64
	MedianWallSeconds    float64
	MinWallSeconds       float64
	MaxWallSeconds       float64
	StddevWallSeconds    float64
	CVPercent            float64
	MeanCPUSeconds       float64
	MeanMaxRSSKB         float64
	AverageCores         float64
	SpeedupVsBaseline    float64
	MeanCgroupCPUSeconds float64
	MeanMemoryPeakBytes  float64
	MeanIOReadBytes      float64
	MeanIOWriteBytes     float64
	CgroupAverageCores   float64
}

func summarizeRuns(runs []runRecord, baseline string) ([]summaryRecord, error) {
	grouped := make(map[string][]runRecord)
	for _, run := range runs {
		if run.Phase == "measured" {
			grouped[run.Variant] = append(grouped[run.Variant], run)
		}
	}
	if len(grouped[baseline]) == 0 {
		return nil, errors.New("缺少 baseline 的 measured 结果")
	}

	variants := make([]string, 0, len(grouped))
	for variant := range grouped {
		variants = append(variants, variant)
	}
	sort.Strings(variants)

	summaries := make([]summaryRecord, 0, len(variants))
	for _, variant := range variants {
		items := grouped[variant]
		walls := make([]float64, 0, len(items))
		var wallSum, cpuSum, rssSum, cgroupCPUSum, memoryPeakSum, ioReadSum, ioWriteSum float64
		for _, item := range items {
			walls = append(walls, item.WallSeconds)
			wallSum += item.WallSeconds
			cpuSum += item.UserSeconds + item.SystemSeconds
			rssSum += float64(item.MaxRSSKB)
			cgroupCPUSum += float64(item.CgroupCPUUserMs+item.CgroupCPUSystemMs) / 1000
			memoryPeakSum += float64(item.MemoryPeakBytes)
			ioReadSum += float64(item.IOReadBytes)
			ioWriteSum += float64(item.IOWriteBytes)
		}
		sort.Float64s(walls)
		mean := wallSum / float64(len(items))
		median := medianFloat64(walls)
		var squared float64
		for _, value := range walls {
			delta := value - mean
			squared += delta * delta
		}
		stddev := math.Sqrt(squared / float64(len(items)))
		averageCores := 0.0
		if wallSum > 0 {
			averageCores = cpuSum / wallSum
		}
		cgroupAverageCores := 0.0
		if wallSum > 0 {
			cgroupAverageCores = cgroupCPUSum / wallSum
		}
		cv := 0.0
		if mean > 0 {
			cv = stddev / mean * 100
		}
		summaries = append(summaries, summaryRecord{
			Variant:              variant,
			Runs:                 len(items),
			MeanWallSeconds:      mean,
			MedianWallSeconds:    median,
			MinWallSeconds:       walls[0],
			MaxWallSeconds:       walls[len(walls)-1],
			StddevWallSeconds:    stddev,
			CVPercent:            cv,
			MeanCPUSeconds:       cpuSum / float64(len(items)),
			MeanMaxRSSKB:         rssSum / float64(len(items)),
			AverageCores:         averageCores,
			MeanCgroupCPUSeconds: cgroupCPUSum / float64(len(items)),
			MeanMemoryPeakBytes:  memoryPeakSum / float64(len(items)),
			MeanIOReadBytes:      ioReadSum / float64(len(items)),
			MeanIOWriteBytes:     ioWriteSum / float64(len(items)),
			CgroupAverageCores:   cgroupAverageCores,
		})
	}

	baselineMedian := 0.0
	for _, item := range summaries {
		if item.Variant == baseline {
			baselineMedian = item.MedianWallSeconds
			break
		}
	}
	if baselineMedian <= 0 {
		return nil, errors.New("baseline wall time 必须大于 0")
	}
	for i := range summaries {
		if summaries[i].MedianWallSeconds > 0 {
			summaries[i].SpeedupVsBaseline = baselineMedian / summaries[i].MedianWallSeconds
		}
	}
	return summaries, nil
}

func medianFloat64(sorted []float64) float64 {
	middle := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[middle]
	}
	return (sorted[middle-1] + sorted[middle]) / 2
}
