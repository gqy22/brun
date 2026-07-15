package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

type optionalInt struct {
	value int
	set   bool
}

type matrixFlags map[string][]string

func (values matrixFlags) String() string { return "" }

func (values matrixFlags) Set(raw string) error {
	key, list, ok := strings.Cut(raw, "=")
	key = strings.TrimSpace(key)
	if !ok || key == "" {
		return fmt.Errorf("matrix 必须使用 name=value1,value2")
	}
	var parsed []string
	for _, value := range strings.Split(list, ",") {
		if value = strings.TrimSpace(value); value != "" {
			parsed = append(parsed, value)
		}
	}
	if len(parsed) == 0 {
		return fmt.Errorf("matrix %s 没有值", key)
	}
	values[key] = parsed
	return nil
}

func (item *optionalInt) String() string {
	if !item.set {
		return ""
	}
	return fmt.Sprintf("%d", item.value)
}

func (item *optionalInt) Set(value string) error {
	var parsed int
	if _, err := fmt.Sscanf(value, "%d", &parsed); err != nil {
		return err
	}
	item.value = parsed
	item.set = true
	return nil
}

func main() {
	var tier, caseRoot, datasetRoot, cacheRoot, workingDir string
	var warmups, repeats optionalInt
	matrices := make(matrixFlags)
	flag.StringVar(&tier, "tier", "smoke", "数据层级")
	flag.StringVar(&caseRoot, "cases", "guide/cases", "实验清单目录")
	flag.StringVar(&datasetRoot, "datasets", "guide/datasets", "数据集清单目录")
	flag.StringVar(&cacheRoot, "cache", "", "guide 数据缓存根目录")
	flag.StringVar(&workingDir, "working-directory", "", "命令工作目录")
	flag.Var(&warmups, "warmups", "覆盖清单中的预热次数")
	flag.Var(&repeats, "repeats", "覆盖清单中的正式重复次数")
	flag.Var(matrices, "matrix", "覆盖参数矩阵，例如 threads=1,2,4（可重复）")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "用法: go run ./guide/cmd/bench [选项] <case-id>\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	if cacheRoot == "" {
		cacheRoot = os.Getenv("BRUN_GUIDE_CACHE")
		if cacheRoot == "" {
			cacheRoot = ".cache/guide-data"
		}
	}
	options := runOptions{
		CaseID:          flag.Arg(0),
		Tier:            tier,
		CaseRoot:        caseRoot,
		DatasetRoot:     datasetRoot,
		CacheRoot:       cacheRoot,
		WorkingDir:      workingDir,
		MatrixOverrides: matrices,
	}
	if warmups.set {
		options.WarmupsOverride = &warmups.value
	}
	if repeats.set {
		options.RepeatsOverride = &repeats.value
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	resultDir, err := runBenchmark(ctx, options)
	if err != nil {
		cancelled := errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled)
		if resultDir != "" {
			if cancelled {
				fmt.Fprintf(os.Stderr, "实验已取消，已保留部分结果目录: %s\n", resultDir)
			} else {
				fmt.Fprintf(os.Stderr, "实验失败，已保留结果目录: %s\n", resultDir)
			}
		}
		fmt.Fprintln(os.Stderr, err)
		if cancelled {
			os.Exit(130)
		}
		os.Exit(1)
	}
	fmt.Printf("benchmark 完成: %s\n", resultDir)
}
