package guide

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

//go:embed content/*/*.md
var contentFS embed.FS

const SchemaVersion = 2

type Versions struct {
	Tested     []string `yaml:"tested"`
	Documented []string `yaml:"documented"`
	Notes      string   `yaml:"notes,omitempty"`
}

type DocumentEvidence struct {
	Title   string `yaml:"title"`
	URL     string `yaml:"url"`
	Checked string `yaml:"checked"`
}

type Evidence struct {
	Docs        []DocumentEvidence `yaml:"docs"`
	Validations []string           `yaml:"validations"`
	Benchmarks  []string           `yaml:"benchmarks"`
}

// Entry is one built-in, versioned piece of operational experience.
type Entry struct {
	Schema     int      `yaml:"schema"`
	ID         string   `yaml:"id"`
	Title      string   `yaml:"title"`
	Tool       string   `yaml:"tool"`
	Category   string   `yaml:"category"`
	Kind       string   `yaml:"kind"`
	Summary    string   `yaml:"summary"`
	Tags       []string `yaml:"tags"`
	Commands   []string `yaml:"commands"`
	Level      string   `yaml:"level"`
	Status     string   `yaml:"status"`
	Versions   Versions `yaml:"versions"`
	Evidence   Evidence `yaml:"evidence"`
	Updated    string   `yaml:"updated"`
	Body       string   `yaml:"-"`
	SourcePath string   `yaml:"-"`
}

var (
	validCategories  = stringSet("workflow", "performance", "parallel", "format", "quality", "troubleshooting", "pitfall")
	validKinds       = stringSet("practice", "comparison", "workflow", "performance", "pitfall", "troubleshooting")
	validLevels      = stringSet("basic", "intermediate", "advanced")
	validStatuses    = stringSet("draft", "reviewed", "verified", "benchmarked", "deprecated")
	requiredSections = []string{
		"结论",
		"适用场景",
		"推荐方法",
		"注意事项",
		"依据",
	}
	stableIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*$`)
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
		"kind": entry.Kind, "level": entry.Level, "status": entry.Status, "updated": entry.Updated,
	}
	for field, value := range required {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("必填字段 %q 为空", field)
		}
	}
	if entry.Schema != SchemaVersion {
		return fmt.Errorf("schema 必须为 %d，当前为 %d", SchemaVersion, entry.Schema)
	}
	if !stableIDPattern.MatchString(entry.ID) || !stableIDPattern.MatchString(entry.Tool) {
		return fmt.Errorf("id 和 tool 只能包含小写字母、数字、点和短横线")
	}
	if !strings.HasPrefix(entry.ID, entry.Tool+".") {
		return fmt.Errorf("id %q 必须以 %q 开头", entry.ID, entry.Tool+".")
	}
	if _, ok := validCategories[entry.Category]; !ok {
		return fmt.Errorf("未知 category %q", entry.Category)
	}
	if _, ok := validKinds[entry.Kind]; !ok {
		return fmt.Errorf("未知 kind %q", entry.Kind)
	}
	if _, ok := validLevels[entry.Level]; !ok {
		return fmt.Errorf("未知 level %q", entry.Level)
	}
	if _, ok := validStatuses[entry.Status]; !ok {
		return fmt.Errorf("未知 status %q", entry.Status)
	}
	updated, err := time.Parse("2006-01-02", entry.Updated)
	if err != nil {
		return fmt.Errorf("updated 必须使用 YYYY-MM-DD: %w", err)
	}
	if err := validateValues("tags", entry.Tags, true); err != nil {
		return err
	}
	if err := validateValues("commands", entry.Commands, false); err != nil {
		return err
	}
	if err := validateValues("versions.tested", entry.Versions.Tested, false); err != nil {
		return err
	}
	if err := validateValues("versions.documented", entry.Versions.Documented, false); err != nil {
		return err
	}
	if err := entry.validateEvidence(updated); err != nil {
		return err
	}
	positions := make(map[string]int)
	for index, line := range strings.Split(entry.Body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "## ") {
			heading := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "## "))
			if _, exists := positions[heading]; exists {
				return fmt.Errorf("正文二级章节 %q 重复", heading)
			}
			positions[heading] = index
		}
	}
	lastPosition := -1
	for _, section := range requiredSections {
		position, ok := positions[section]
		if !ok {
			return fmt.Errorf("正文缺少必需章节 %q", section)
		}
		if position < lastPosition {
			return fmt.Errorf("正文必需章节 %q 顺序不正确", section)
		}
		lastPosition = position
	}
	return nil
}

func validateValues(field string, values []string, required bool) error {
	if required && len(values) == 0 {
		return fmt.Errorf("%s 至少需要一项", field)
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return fmt.Errorf("%s 不能包含空值", field)
		}
		if _, exists := seen[value]; exists {
			return fmt.Errorf("%s 包含重复值 %q", field, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func (entry Entry) validateEvidence(updated time.Time) error {
	if err := validateValues("evidence.validations", entry.Evidence.Validations, false); err != nil {
		return err
	}
	if err := validateValues("evidence.benchmarks", entry.Evidence.Benchmarks, false); err != nil {
		return err
	}
	for _, id := range append(append([]string{}, entry.Evidence.Validations...), entry.Evidence.Benchmarks...) {
		if !stableIDPattern.MatchString(id) {
			return fmt.Errorf("无效的证据 ID %q", id)
		}
	}
	docURLs := make(map[string]struct{}, len(entry.Evidence.Docs))
	for _, doc := range entry.Evidence.Docs {
		if strings.TrimSpace(doc.Title) == "" {
			return fmt.Errorf("evidence.docs.title 不能为空")
		}
		parsed, err := url.ParseRequestURI(doc.URL)
		if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") {
			return fmt.Errorf("无效的文档 URL %q", doc.URL)
		}
		checked, err := time.Parse("2006-01-02", doc.Checked)
		if err != nil {
			return fmt.Errorf("evidence.docs.checked 必须使用 YYYY-MM-DD: %w", err)
		}
		if checked.After(updated) {
			return fmt.Errorf("文档核对日期 %s 晚于 updated %s", doc.Checked, entry.Updated)
		}
		if _, exists := docURLs[doc.URL]; exists {
			return fmt.Errorf("evidence.docs 包含重复 URL %q", doc.URL)
		}
		docURLs[doc.URL] = struct{}{}
	}
	switch entry.Status {
	case "reviewed":
		if len(entry.Evidence.Docs) == 0 || len(entry.Versions.Documented) == 0 {
			return fmt.Errorf("reviewed 状态需要官方文档证据和 documented 版本")
		}
	case "verified":
		if len(entry.Evidence.Docs) == 0 || len(entry.Evidence.Validations) == 0 || len(entry.Versions.Tested) == 0 || len(entry.Versions.Documented) == 0 {
			return fmt.Errorf("verified 状态需要文档、正确性验证、tested 和 documented 版本")
		}
	case "benchmarked":
		if len(entry.Evidence.Docs) == 0 || len(entry.Evidence.Validations) == 0 || len(entry.Evidence.Benchmarks) == 0 || len(entry.Versions.Tested) == 0 || len(entry.Versions.Documented) == 0 {
			return fmt.Errorf("benchmarked 状态需要文档、正确性验证、性能报告、tested 和 documented 版本")
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
			entry.ID, entry.Title, entry.Tool, entry.Category, entry.Kind, entry.Summary,
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
