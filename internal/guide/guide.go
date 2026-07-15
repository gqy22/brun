package guide

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

//go:embed content/*/*.md
var contentFS embed.FS

// ToolVersions records the versions for which an entry was tested and is
// expected to apply. Applicability is guidance, not a dependency constraint.
type ToolVersions struct {
	Tested     []string `yaml:"tested"`
	Applicable string   `yaml:"applicable"`
}

// Entry is one built-in, versioned piece of operational experience.
type Entry struct {
	ID           string       `yaml:"id"`
	Title        string       `yaml:"title"`
	Tool         string       `yaml:"tool"`
	Category     string       `yaml:"category"`
	Summary      string       `yaml:"summary"`
	Tags         []string     `yaml:"tags"`
	Commands     []string     `yaml:"commands"`
	Level        string       `yaml:"level"`
	Status       string       `yaml:"status"`
	ToolVersions ToolVersions `yaml:"tool_versions"`
	Updated      string       `yaml:"updated"`
	Body         string       `yaml:"-"`
	SourcePath   string       `yaml:"-"`
}

var (
	validCategories  = stringSet("workflow", "performance", "parallel", "format", "quality", "troubleshooting", "pitfall")
	validLevels      = stringSet("basic", "intermediate", "advanced")
	validStatuses    = stringSet("draft", "tested", "verified", "benchmarked", "deprecated")
	requiredSections = []string{
		"## 结论",
		"## 适用场景",
		"## 推荐命令",
		"## 为什么这样做",
		"## 并行与资源",
		"## 注意事项",
		"## 结果检查",
		"## 依据",
	}
)

func stringSet(values ...string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

// Load reads and validates every guide entry embedded in the brun binary.
func Load() ([]Entry, error) {
	paths, err := fs.Glob(contentFS, "content/*/*.md")
	if err != nil {
		return nil, fmt.Errorf("查找内置指南: %w", err)
	}
	entries := make([]Entry, 0, len(paths))
	seen := make(map[string]string, len(paths))
	for _, path := range paths {
		data, err := contentFS.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("读取内置指南 %s: %w", path, err)
		}
		entry, err := Parse(data)
		if err != nil {
			return nil, fmt.Errorf("解析内置指南 %s: %w", path, err)
		}
		entry.SourcePath = path
		if previous, ok := seen[entry.ID]; ok {
			return nil, fmt.Errorf("指南 ID %q 重复: %s 和 %s", entry.ID, previous, path)
		}
		seen[entry.ID] = path
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	return entries, nil
}

// Parse parses and validates one Markdown guide document with YAML frontmatter.
func Parse(data []byte) (Entry, error) {
	var entry Entry
	frontmatter, body, err := splitFrontmatter(data)
	if err != nil {
		return entry, err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(frontmatter))
	decoder.KnownFields(true)
	if err := decoder.Decode(&entry); err != nil {
		return entry, fmt.Errorf("无效元数据: %w", err)
	}
	entry.Body = strings.TrimSpace(string(body))
	if err := entry.Validate(); err != nil {
		return Entry{}, err
	}
	return entry, nil
}

func splitFrontmatter(data []byte) ([]byte, []byte, error) {
	normalized := strings.ReplaceAll(string(data), "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return nil, nil, fmt.Errorf("缺少 YAML frontmatter 起始标记 ---")
	}
	end := strings.Index(normalized[4:], "\n---\n")
	if end < 0 {
		return nil, nil, fmt.Errorf("缺少 YAML frontmatter 结束标记 ---")
	}
	end += 4
	return []byte(normalized[4:end]), []byte(normalized[end+5:]), nil
}

// Validate enforces the stable authoring contract for built-in content.
func (entry Entry) Validate() error {
	required := map[string]string{
		"id": entry.ID, "title": entry.Title, "tool": entry.Tool,
		"category": entry.Category, "summary": entry.Summary,
		"level": entry.Level, "status": entry.Status, "updated": entry.Updated,
	}
	for field, value := range required {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("必填字段 %q 为空", field)
		}
	}
	if !strings.HasPrefix(entry.ID, entry.Tool+".") {
		return fmt.Errorf("id %q 必须以 %q 开头", entry.ID, entry.Tool+".")
	}
	if _, ok := validCategories[entry.Category]; !ok {
		return fmt.Errorf("未知 category %q", entry.Category)
	}
	if _, ok := validLevels[entry.Level]; !ok {
		return fmt.Errorf("未知 level %q", entry.Level)
	}
	if _, ok := validStatuses[entry.Status]; !ok {
		return fmt.Errorf("未知 status %q", entry.Status)
	}
	if _, err := time.Parse("2006-01-02", entry.Updated); err != nil {
		return fmt.Errorf("updated 必须使用 YYYY-MM-DD: %w", err)
	}
	if len(entry.Tags) == 0 {
		return fmt.Errorf("tags 至少需要一项")
	}
	if len(entry.ToolVersions.Tested) == 0 || strings.TrimSpace(entry.ToolVersions.Applicable) == "" {
		return fmt.Errorf("tool_versions.tested 和 tool_versions.applicable 不能为空")
	}
	for _, section := range requiredSections {
		if !strings.Contains(entry.Body, section) {
			return fmt.Errorf("正文缺少必需章节 %q", section)
		}
	}
	return nil
}

// Find returns an entry by its stable ID.
func Find(entries []Entry, id string) (Entry, bool) {
	for _, entry := range entries {
		if entry.ID == id {
			return entry, true
		}
	}
	return Entry{}, false
}

// Filter returns entries matching all non-empty structured filters.
func Filter(entries []Entry, tool, category string) []Entry {
	var matches []Entry
	for _, entry := range entries {
		if tool != "" && !strings.EqualFold(entry.Tool, tool) {
			continue
		}
		if category != "" && !strings.EqualFold(entry.Category, category) {
			continue
		}
		matches = append(matches, entry)
	}
	return matches
}

// Search performs a simple case-insensitive full-text search over metadata and body.
func Search(entries []Entry, query string) []Entry {
	words := strings.Fields(strings.ToLower(query))
	if len(words) == 0 {
		return nil
	}
	var matches []Entry
	for _, entry := range entries {
		haystack := strings.ToLower(strings.Join([]string{
			entry.ID, entry.Title, entry.Tool, entry.Category, entry.Summary,
			strings.Join(entry.Tags, " "), strings.Join(entry.Commands, " "), entry.Body,
		}, "\n"))
		matched := true
		for _, word := range words {
			if !strings.Contains(haystack, word) {
				matched = false
				break
			}
		}
		if matched {
			matches = append(matches, entry)
		}
	}
	return matches
}
