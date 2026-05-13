package internal

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Project string        `yaml:"project"`
	Capture CaptureConfig `yaml:"capture"`
	Ignore  []string      `yaml:"ignore"`
}

type CaptureConfig struct {
	Scripts []string `yaml:"scripts"`
	Configs []string `yaml:"configs"`
	Outputs []string `yaml:"outputs"`
}

func ParseConfig(data []byte) (Config, error) {
	var cfg Config
	err := yaml.Unmarshal(data, &cfg)
	return cfg, err
}

type FileInfo struct {
	Path    string
	Size    int64
	ModTime int64
	IsDir   bool
}

func SnapshotDir(root string, ignorePatterns []string) (map[string]FileInfo, error) {
	result := make(map[string]FileInfo)

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if rel == "." {
			return nil
		}
		if matchesAny(rel, ignorePatterns) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		result[rel] = FileInfo{
			Path:    rel,
			Size:    info.Size(),
			ModTime: info.ModTime().UnixNano(),
			IsDir:   info.IsDir(),
		}
		return nil
	})

	return result, err
}

func DiffSnapshots(before, after map[string]FileInfo) (created, modified, deleted []FileInfo) {
	for path, info := range after {
		if info.Path == "" {
			info.Path = path
		}
		if old, exists := before[path]; !exists {
			created = append(created, info)
		} else if old.Size != info.Size || old.ModTime != info.ModTime {
			modified = append(modified, info)
		}
	}
	for path, info := range before {
		if info.Path == "" {
			info.Path = path
		}
		if _, exists := after[path]; !exists {
			deleted = append(deleted, info)
		}
	}
	return
}

func ClassifyArtifact(path string) string {
	lower := strings.ToLower(path)
	ext := strings.ToLower(filepath.Ext(path))

	// 处理复合扩展名
	if strings.HasSuffix(lower, ".fastq.gz") || strings.HasSuffix(lower, ".fq.gz") {
		return "input"
	}
	if strings.HasSuffix(lower, ".vcf.gz") {
		return "output"
	}

	switch ext {
	case ".py", ".r", ".sh", ".nf":
		return "script"
	case ".yaml", ".yml", ".json", ".toml", ".conf", ".ini":
		return "config"
	case ".html", ".htm":
		return "report"
	case ".fastq", ".fq", ".bam", ".cram", ".sam":
		if strings.Contains(lower, "/data/") || strings.Contains(lower, "/input/") {
			return "input"
		}
		return "output"
	default:
		return "output"
	}
}

func IsLargeFile(size int64) bool {
	const largeThreshold = 50 * 1024 * 1024 // 50MB
	return size > largeThreshold
}

func matchesAny(path string, patterns []string) bool {
	for _, p := range patterns {
		matched, err := filepath.Match(p, path)
		if err == nil && matched {
			return true
		}
		// 支持 ** 通配符
		if strings.HasSuffix(p, "/**") {
			prefix := strings.TrimSuffix(p, "**")
			if strings.HasPrefix(path, prefix) {
				return true
			}
		}
	}
	return false
}
