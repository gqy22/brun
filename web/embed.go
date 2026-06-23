package web

import (
	"embed"
	"io/fs"
)

//go:embed templates/*
var templatesRaw embed.FS

//go:embed static/*
var staticRaw embed.FS

// Templates / Static 已剥离 embed 前缀（templates/、static/），FS 根即内容目录，
// 可直接按文件名读取，供 renderTemplate 的 fs.ReadFile(name) 和 http.FileServer 使用。
// fs.Sub 对 embed.FS 的既有子目录不会出错，故忽略返回的 error。
var Templates, _ = fs.Sub(templatesRaw, "templates")
var Static, _ = fs.Sub(staticRaw, "static")
