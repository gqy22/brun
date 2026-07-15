package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestGuideListUsesReadableStackedLayout(t *testing.T) {
	c := guideListCmd()
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetErr(&out)
	c.SetArgs([]string{"--tool", "bcftools"})

	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{
		"bcftools.pipeline-uncompressed",
		"bcftools.parallel-by-contig",
		"bcftools · performance · benchmarked",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("guide list missing %q:\n%s", want, got)
		}
	}
}

func TestGuideShowRendersMarkdownForTerminal(t *testing.T) {
	c := guideShowCmd()
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetErr(&out)
	c.SetArgs([]string{"bcftools.pipeline-uncompressed"})

	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{"管道中间使用未压缩 BCF", "结论:", "bcftools view -Ou"} {
		if !strings.Contains(got, want) {
			t.Fatalf("guide show missing %q:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{"## 结论", "```bash"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("guide show contains raw Markdown %q:\n%s", unwanted, got)
		}
	}
}

func TestGuideSearchRequiresAllWords(t *testing.T) {
	c := guideSearchCmd()
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetErr(&out)
	c.SetArgs([]string{"并行 contig"})

	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "bcftools.parallel-by-contig") || strings.Contains(got, "pipeline-uncompressed") {
		t.Fatalf("unexpected search output:\n%s", got)
	}
}
