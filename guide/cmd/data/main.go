package main

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type checksum struct {
	Algorithm string `yaml:"algorithm"`
	Value     string `yaml:"value"`
}

type artifact struct {
	Format    string     `yaml:"format"`
	URL       string     `yaml:"url"`
	Filename  string     `yaml:"filename"`
	Bytes     int64      `yaml:"bytes"`
	Checksums []checksum `yaml:"checksums"`
}

type dataset struct {
	ID          string    `yaml:"id"`
	Tier        string    `yaml:"tier"`
	Description string    `yaml:"description"`
	Source      artifact  `yaml:"source"`
	Index       *artifact `yaml:"index"`
}

func main() {
	var tier, wanted, cache, manifestRoot string
	flag.StringVar(&tier, "tier", "correctness", "要下载的数据级别")
	flag.StringVar(&wanted, "dataset", "", "只下载指定数据集 ID")
	flag.StringVar(&cache, "cache", "", "缓存根目录")
	flag.StringVar(&manifestRoot, "manifests", "guide/datasets", "数据清单目录")
	flag.Parse()

	if cache == "" {
		if cache = os.Getenv("BRUN_GUIDE_CACHE"); cache == "" {
			cache = filepath.Join(".cache", "guide-data")
		}
	}
	datasets, err := loadDatasets(manifestRoot)
	if err != nil {
		fatal(err)
	}
	selected := selectDatasets(datasets, tier, wanted)
	if len(selected) == 0 {
		fatal(fmt.Errorf("没有找到 tier=%q dataset=%q 的数据清单", tier, wanted))
	}
	client := &http.Client{Timeout: 0}
	for _, ds := range selected {
		fmt.Printf("数据集: %s (%s)\n", ds.ID, ds.Tier)
		dir := filepath.Join(cache, "downloads", ds.ID)
		if err := downloadArtifact(context.Background(), client, dir, ds.Source); err != nil {
			fatal(fmt.Errorf("下载 %s: %w", ds.ID, err))
		}
		if ds.Index != nil {
			if err := downloadArtifact(context.Background(), client, dir, *ds.Index); err != nil {
				fatal(fmt.Errorf("下载 %s 索引: %w", ds.ID, err))
			}
		}
	}
}

func loadDatasets(root string) ([]dataset, error) {
	var datasets []dataset
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || (!strings.HasSuffix(path, ".yaml") && !strings.HasSuffix(path, ".yml")) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var ds dataset
		if err := yaml.Unmarshal(data, &ds); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if err := validateDataset(ds); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		datasets = append(datasets, ds)
		return nil
	})
	sort.Slice(datasets, func(i, j int) bool { return datasets[i].ID < datasets[j].ID })
	return datasets, err
}

func validateDataset(ds dataset) error {
	if ds.ID == "" || ds.Tier == "" || ds.Source.URL == "" || ds.Source.Filename == "" {
		return errors.New("id、tier 和 source 的 url/filename 不能为空")
	}
	if ds.Tier != "correctness" && ds.Tier != "smoke" && ds.Tier != "medium" {
		return fmt.Errorf("未知 tier %q", ds.Tier)
	}
	for _, item := range append([]artifact{ds.Source}, optionalArtifact(ds.Index)...) {
		if item.Bytes <= 0 || len(item.Checksums) == 0 {
			return fmt.Errorf("artifact %q 缺少 bytes 或 checksums", item.Filename)
		}
		for _, sum := range item.Checksums {
			if _, err := newHash(sum.Algorithm); err != nil {
				return err
			}
			if strings.TrimSpace(sum.Value) == "" {
				return fmt.Errorf("artifact %q 的 %s 校验值为空", item.Filename, sum.Algorithm)
			}
		}
	}
	return nil
}

func optionalArtifact(item *artifact) []artifact {
	if item == nil {
		return nil
	}
	return []artifact{*item}
}

func selectDatasets(datasets []dataset, tier, wanted string) []dataset {
	var selected []dataset
	for _, ds := range datasets {
		if wanted != "" {
			if ds.ID == wanted {
				selected = append(selected, ds)
			}
			continue
		}
		if ds.Tier == tier {
			selected = append(selected, ds)
		}
	}
	return selected
}

func downloadArtifact(ctx context.Context, client *http.Client, dir string, item artifact) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	destination := filepath.Join(dir, item.Filename)
	if ok, _ := verifyArtifact(destination, item); ok {
		fmt.Printf("  使用已校验缓存: %s\n", destination)
		return nil
	}
	partial := destination + ".part"
	if ok, _ := verifyArtifact(partial, item); ok {
		if err := os.Rename(partial, destination); err != nil {
			return err
		}
		fmt.Printf("  已校验并采用完整断点文件: %s\n", destination)
		return nil
	}
	var offset int64
	if info, err := os.Stat(partial); err == nil {
		offset = info.Size()
		if offset >= item.Bytes {
			offset = 0
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, item.URL, nil)
	if err != nil {
		return err
	}
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	flags := os.O_CREATE | os.O_WRONLY
	if offset > 0 && resp.StatusCode == http.StatusPartialContent {
		flags |= os.O_APPEND
	} else {
		offset = 0
		flags |= os.O_TRUNC
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("HTTP %s", resp.Status)
	}
	file, err := os.OpenFile(partial, flags, 0o644)
	if err != nil {
		return err
	}
	fmt.Printf("  下载 %s (%s)，从 %s 开始\n", item.Filename, formatBytes(item.Bytes), formatBytes(offset))
	progress := &progressWriter{total: item.Bytes, written: offset, last: time.Now()}
	_, copyErr := io.Copy(io.MultiWriter(file, progress), resp.Body)
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	fmt.Printf("\r  下载完成: %s                    \n", formatBytes(item.Bytes))
	if ok, reason := verifyArtifact(partial, item); !ok {
		return fmt.Errorf("下载文件校验失败: %s", reason)
	}
	if err := os.Rename(partial, destination); err != nil {
		return err
	}
	return nil
}

type progressWriter struct {
	total, written int64
	last           time.Time
}

func (writer *progressWriter) Write(data []byte) (int, error) {
	writer.written += int64(len(data))
	if time.Since(writer.last) >= time.Second {
		percent := float64(writer.written) / float64(writer.total) * 100
		fmt.Printf("\r  进度: %6.2f%%  %s / %s", percent, formatBytes(writer.written), formatBytes(writer.total))
		writer.last = time.Now()
	}
	return len(data), nil
}

func verifyArtifact(path string, item artifact) (bool, string) {
	info, err := os.Stat(path)
	if err != nil {
		return false, err.Error()
	}
	if info.Size() != item.Bytes {
		return false, fmt.Sprintf("大小为 %d，预期 %d", info.Size(), item.Bytes)
	}
	for _, sum := range item.Checksums {
		digest, err := fileChecksum(path, sum.Algorithm)
		if err != nil {
			return false, err.Error()
		}
		if !strings.EqualFold(digest, sum.Value) {
			return false, fmt.Sprintf("%s 为 %s，预期 %s", sum.Algorithm, digest, sum.Value)
		}
	}
	return true, ""
}

func fileChecksum(path, algorithm string) (string, error) {
	digest, err := newHash(algorithm)
	if err != nil {
		return "", err
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	if _, err := io.Copy(digest, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func newHash(algorithm string) (hash.Hash, error) {
	switch strings.ToLower(algorithm) {
	case "md5":
		return md5.New(), nil
	case "sha256":
		return sha256.New(), nil
	default:
		return nil, fmt.Errorf("不支持的校验算法 %q", algorithm)
	}
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	value := float64(bytes)
	units := []string{"KiB", "MiB", "GiB", "TiB"}
	for _, suffix := range units {
		value /= unit
		if value < unit {
			return fmt.Sprintf("%.1f %s", value, suffix)
		}
	}
	return fmt.Sprintf("%.1f PiB", value/unit)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "错误:", err)
	os.Exit(1)
}
