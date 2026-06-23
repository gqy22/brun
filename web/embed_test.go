package web

import (
	"io/fs"
	"testing"
)

// TestTemplatesAccessibleWithoutPrefix 防止 embed 前缀回归：
// FS 必须能直接按文件名读取模板/静态资源（剥离 templates/、static/ 前缀后）。
func TestTemplatesAccessibleWithoutPrefix(t *testing.T) {
	for _, name := range []string{"index.html", "run.html", "layout.html"} {
		if _, err := fs.ReadFile(Templates, name); err != nil {
			t.Errorf("ReadFile(Templates, %q) 失败: %v（embed 前缀是否已剥离?）", name, err)
		}
	}
	if _, err := fs.ReadFile(Static, "app.js"); err != nil {
		t.Errorf("ReadFile(Static, \"app.js\") 失败: %v（embed 前缀是否已剥离?）", err)
	}
}
