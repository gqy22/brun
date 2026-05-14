package internal

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

var globalLog *slog.Logger

func init() {
	globalLog = slog.Default()
}

func InitLogger() error {
	logPath := filepath.Join(HomeDir(), "brun.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}

	multi := io.MultiWriter(os.Stdout, f)
	handler := slog.NewTextHandler(multi, &slog.HandlerOptions{
		Level: slog.LevelInfo,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				a.Value = slog.StringValue(a.Value.Time().Format("2006-01-02 15:04:05"))
			}
			if a.Key == slog.LevelKey {
				l := a.Value.String()
				switch l {
				case "INFO":
					a.Value = slog.StringValue("INFO ")
				case "WARN":
					a.Value = slog.StringValue("WARN ")
				case "ERROR":
					a.Value = slog.StringValue("ERROR")
				}
			}
			return a
		},
	})
	globalLog = slog.New(handler)
	return nil
}

func Log() *slog.Logger { return globalLog }

// LogDuration 记录耗时（用于 run 完成时）
func LogDuration(start time.Time) string {
	d := time.Since(start)
	if d < time.Second {
		return d.Round(time.Millisecond).String()
	}
	if d < time.Hour {
		return d.Round(time.Second).String()
	}
	return d.Round(time.Minute).String()
}
