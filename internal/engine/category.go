package engine

import (
	"path/filepath"
	"strings"
)

// The categories a download can have. The page labels them in Chinese; the
// directories they make keep these English names.
const (
	CategoryVideo    = "video"
	CategoryAudio    = "audio"
	CategoryArchive  = "archive"
	CategoryDocument = "document"
	CategoryApp      = "app"
	CategoryOther    = "other"
)

var categoryByExt = map[string]string{
	"mp4": CategoryVideo, "mkv": CategoryVideo, "avi": CategoryVideo,
	"mov": CategoryVideo, "webm": CategoryVideo, "flv": CategoryVideo,
	"ts": CategoryVideo, "m4v": CategoryVideo, "wmv": CategoryVideo,

	"mp3": CategoryAudio, "flac": CategoryAudio, "wav": CategoryAudio,
	"aac": CategoryAudio, "m4a": CategoryAudio, "ogg": CategoryAudio,
	"opus": CategoryAudio,

	"zip": CategoryArchive, "rar": CategoryArchive, "7z": CategoryArchive,
	"tar": CategoryArchive, "gz": CategoryArchive, "bz2": CategoryArchive,
	"xz": CategoryArchive, "iso": CategoryArchive, "zst": CategoryArchive,

	"pdf": CategoryDocument, "doc": CategoryDocument, "docx": CategoryDocument,
	"xls": CategoryDocument, "xlsx": CategoryDocument, "ppt": CategoryDocument,
	"pptx": CategoryDocument, "txt": CategoryDocument, "epub": CategoryDocument,
	"md": CategoryDocument,

	"exe": CategoryApp, "msi": CategoryApp, "msix": CategoryApp,
	"apk": CategoryApp, "dmg": CategoryApp, "pkg": CategoryApp,
	"deb": CategoryApp, "appx": CategoryApp,
}

var categoryFolders = map[string]string{
	CategoryVideo:    "Video",
	CategoryAudio:    "Audio",
	CategoryArchive:  "Archives",
	CategoryDocument: "Documents",
	CategoryApp:      "Apps",
	CategoryOther:    "Other",
}

// categoryFor guesses the category from the file's extension.
func categoryFor(name string) string {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")
	if cat, ok := categoryByExt[ext]; ok {
		return cat
	}
	return CategoryOther
}

func validCategory(c string) bool {
	_, ok := categoryFolders[c]
	return ok
}

// categoryDir is the directory a category gets under the download folder.
func categoryDir(c string) string {
	if dir, ok := categoryFolders[c]; ok {
		return dir
	}
	return categoryFolders[CategoryOther]
}
