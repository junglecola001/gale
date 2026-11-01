package netx

import "mime"

// FilenameFromDisposition returns the file name a Content-Disposition header
// names, or "" when it names none. mime.ParseMediaType decodes RFC 2231's
// filename* (UTF-8 percent escapes) into the filename parameter.
func FilenameFromDisposition(v string) string {
	if v == "" {
		return ""
	}
	_, params, err := mime.ParseMediaType(v)
	if err != nil {
		return ""
	}
	return params["filename"]
}
